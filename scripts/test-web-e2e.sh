#!/usr/bin/env bash
set -euo pipefail

repo_root="$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")/.." && pwd)"
for tool in docker go npm curl python3; do
  command -v "$tool" >/dev/null
done
test_dir="$(mktemp -d)"
postgres_id=""
api_pid=""
cleanup() {
  result=$?
  trap - EXIT
  if [[ -n "$api_pid" ]]; then
    kill "$api_pid" 2>/dev/null || true
    wait "$api_pid" 2>/dev/null || true
  fi
  if [[ -n "$postgres_id" ]]; then docker stop "$postgres_id" >/dev/null || true; fi
  rm -rf -- "$test_dir"
  exit "$result"
}
trap cleanup EXIT

postgres_id="$(docker run --detach --rm --network bridge -e POSTGRES_USER=museum -e POSTGRES_PASSWORD=museum -e POSTGRES_DB=museum_test postgres:16-alpine)"
for attempt in {1..30}; do
  if docker exec "$postgres_id" pg_isready -h 127.0.0.1 -U museum -d museum_test >/dev/null; then break; fi
  if [[ "$attempt" == 30 ]]; then echo 'Test database did not become ready' >&2; exit 1; fi
  sleep 1
done
postgres_host="$(docker inspect --format '{{range .NetworkSettings.Networks}}{{.IPAddress}}{{end}}' "$postgres_id")"
test -n "$postgres_host"
export DATABASE_URL="postgres://museum:museum@$postgres_host:5432/museum_test?sslmode=disable"
api_port="$(python3 -c 'import socket; s=socket.socket(); s.bind(("127.0.0.1", 0)); print(s.getsockname()[1]); s.close()')"
export API_ADDR="127.0.0.1:$api_port"
export PUBLIC_BASE_URL="http://$API_ADDR"
export DEV_AUTH_ENABLED=true
# This isolated fixture exercises public suggestion intake; production defaults off.
export PUBLIC_ROUTE_BRIEF_INTAKE_ENABLED="${PUBLIC_ROUTE_BRIEF_INTAKE_ENABLED:-true}"
export ADMIN_BOOTSTRAP_EMAILS=admin@example.com
export MEDIA_FS_DIR="$test_dir/media"
cd "$repo_root/backend"
go run ./cmd/migrate up
go build -o "$test_dir/api" ./cmd/api
"$test_dir/api" >"$test_dir/api.log" 2>&1 &
api_pid=$!
for attempt in {1..30}; do
  if curl --fail --silent "$PUBLIC_BASE_URL/healthz" >/dev/null; then break; fi
  if [[ "$attempt" == 30 ]] || ! kill -0 "$api_pid" 2>/dev/null; then
    tail -40 "$test_dir/api.log" >&2
    exit 1
  fi
  sleep 1
done
cd "$repo_root/web"
E2E_API_URL="$PUBLIC_BASE_URL" npm run test:e2e -- "$@"
