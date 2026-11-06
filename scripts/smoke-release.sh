#!/usr/bin/env bash
# Exercise the shipped images with disposable resources, including real migrations.
set -euo pipefail
service=${1:?service required}
image=${2:?image required}
revision=${3:?revision required}
scratch=$(mktemp -d)
containers=()
network="byl-smoke-$(basename "$scratch")"
cleanup() {
  for container in "${containers[@]}"; do docker rm -f "$container" >/dev/null 2>&1 || true; done
  docker network rm "$network" >/dev/null 2>&1 || true
  rm -rf "$scratch"
}
trap cleanup EXIT
docker network create "$network" >/dev/null

if [[ $service == api ]]; then
  postgres=$(docker run --rm -d --network "$network" --network-alias postgres \
    -e POSTGRES_USER=byl -e POSTGRES_PASSWORD=smoke-only -e POSTGRES_DB=byl postgres:16-alpine)
  containers+=("$postgres")
  ready=false
  for _ in {1..60}; do
    if docker exec "$postgres" pg_isready -U byl -d byl >/dev/null 2>&1; then ready=true; break; fi
    sleep 1
  done
  [[ $ready == true ]]
  database_url='postgres://byl:smoke-only@postgres:5432/byl?sslmode=disable'
  docker run --rm --network "$network" -e "DATABASE_URL=$database_url" \
    -e MIGRATIONS_DIR=/migrations --entrypoint /migrate "$image" up
  container=$(docker run --rm -d --network "$network" -e "DATABASE_URL=$database_url" \
    -e DEV_AUTH_ENABLED=false "$image")
  port=8080
elif [[ $service == web ]]; then
  # Web smoke checks its static artifact; API behavior was checked above.
  container=$(docker run --rm -d --network "$network" -e API_UPSTREAM=http://127.0.0.1:8080 "$image")
  port=80
else
  echo 'Unknown service' >&2
  exit 2
fi
containers+=("$container")
host=$(docker inspect --format '{{range .NetworkSettings.Networks}}{{.IPAddress}}{{end}}' "$container")
ready=false
for _ in {1..60}; do
  if curl --max-time 3 --fail --silent "http://$host:$port/" >/dev/null; then ready=true; break; fi
  # The API intentionally has no root route.
  if [[ $service == api ]] && curl --max-time 3 --fail --silent "http://$host:$port/healthz" >/dev/null; then ready=true; break; fi
  sleep 1
done
[[ $ready == true ]]
if [[ $service == api ]]; then
  curl --fail --silent "http://$host:$port/api/v1/cities" > "$scratch/cities.json"
  python3 -c 'import json,sys; assert any(c["slug"] == "downers-grove" for c in json.load(open(sys.argv[1]))["items"])' "$scratch/cities.json"
  status=$(curl --silent -o /dev/null -w '%{http_code}' -H 'Content-Type: application/json' \
    -d '{"email":"release-smoke@example.invalid"}' "http://$host:$port/api/v1/auth/dev")
  [[ $status == 404 ]]
else
  curl --fail --silent "http://$host:$port/release.json" > "$scratch/release.json"
  python3 -c 'import json,sys; assert json.load(open(sys.argv[1])) == {"revision":sys.argv[2]}' "$scratch/release.json" "$revision"
  curl --fail --silent "http://$host:$port/" > "$scratch/index.html"
  grep -q 'Byl' "$scratch/index.html"
fi
echo "$service release image smoke test passed"
