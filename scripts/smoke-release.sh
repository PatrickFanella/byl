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
    if docker exec "$postgres" pg_isready -h 127.0.0.1 -U byl -d byl >/dev/null 2>&1; then ready=true; break; fi
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
ready=false
path=/
if [[ $service == api ]]; then path=/healthz; fi
for _ in {1..60}; do
  # The CI job is on another Docker bridge. Probe inside the shipped container
  # rather than depending on cross-bridge forwarding on the runner host.
  if docker exec "$container" wget -T 3 -q -O /dev/null "http://127.0.0.1:$port$path"; then ready=true; break; fi
  sleep 1
done
[[ $ready == true ]]
if [[ $service == api ]]; then
  docker exec "$container" wget -q -O - "http://127.0.0.1:$port/api/v1/cities" > "$scratch/cities.json"
  python3 -c 'import json,sys; assert any(c["slug"] == "downers-grove" for c in json.load(open(sys.argv[1]))["items"])' "$scratch/cities.json"
  docker exec "$container" wget -q -O - "http://127.0.0.1:$port/api/v1/itinerary-capabilities" > "$scratch/itinerary-capabilities.json"
  python3 -c 'import json,sys; c=json.load(open(sys.argv[1])); assert c == {"scheduled_transit_v1":False,"provider":"cta_gtfs_blue_v1","live":False,"transfers":False,"bicycles":False}' "$scratch/itinerary-capabilities.json"
  if docker exec "$container" wget -S -O /dev/null --header 'Content-Type: application/json' \
    --post-data '{"email":"release-smoke@example.invalid"}' "http://127.0.0.1:$port/api/v1/auth/dev" 2> "$scratch/auth-response"; then
    echo 'Development sign-in must be disabled' >&2
    exit 1
  fi
  grep -q '404 Not Found' "$scratch/auth-response"
  # This key is a public, disposable smoke fixture, never a runtime credential.
  smoke_scope_key=3031323334353637383961626364656630313233343536373839616263646566
  docker run --rm --network "$network" -e "DATABASE_URL=$database_url" \
    -e "ROUTE_CONTRIBUTION_SCOPE_KEY=$smoke_scope_key" -e ROUTE_CONTRIBUTION_CUSTODY_APPROVED=true \
    --entrypoint /contribution-retention "$image" --admit-custody
  # Synthetic private rows exercise the shipped cleanup binary and the same
  # table-data exclusions as the operator-installed backup hook. These SQL-only
  # fixtures make no claim about reviewed route access or detector calibration.
  docker exec -i "$postgres" psql -U byl -d byl -v ON_ERROR_STOP=1 >/dev/null <<'SQL'
WITH consent AS (
 INSERT INTO route_contribution_consents(owner_scope,tour_id,revision_id,mode,grant_key_hash,request_hash,deletion_hash,requested_on,created_on,expires_on)
 SELECT repeat('1',64),tour_id,id,'walk',repeat('2',64),repeat('3',64),repeat('4',64),CURRENT_DATE,CURRENT_DATE,CURRENT_DATE+30
 FROM tour_revisions ORDER BY id LIMIT 1 RETURNING id,owner_scope
)
INSERT INTO route_contributions(consent_id,owner_scope,segment_id,direction,observation_period,schema_version,detector_version,calibrated,qualified_passages,sustained_departure_rejoins,submission_key_hash,request_hash,expires_on)
SELECT id,owner_scope,'synthetic-backup','forward',CURRENT_DATE,1,'uncalibrated-v1',false,1,0,repeat('5',64),repeat('6',64),CURRENT_DATE+30 FROM consent;
INSERT INTO route_contribution_limits(scope,count,expires_on) VALUES(repeat('7',64),1,CURRENT_DATE+2);
UPDATE route_contribution_custody SET admitted_on=CURRENT_DATE-2;
SQL
  original_private=$(docker exec "$postgres" psql -U byl -d byl -Atqc 'SELECT count(*) FROM route_contributions')
  [[ $original_private == 1 ]]
  docker exec "$postgres" pg_dump -U byl -d byl -Fc \
    --exclude-table-data='*.route_contribution_consents' \
    --exclude-table-data='*.route_contributions' \
    --exclude-table-data='*.route_contribution_limits' \
    --exclude-table-data='*.route_contribution_custody' > "$scratch/filtered.dump"
  docker exec "$postgres" createdb -U byl byl_restore
  docker exec -i "$postgres" pg_restore -U byl -d byl_restore --exit-on-error < "$scratch/filtered.dump"
  restored_private=$(docker exec "$postgres" psql -U byl -d byl_restore -Atqc 'SELECT (SELECT count(*) FROM route_contributions)+(SELECT count(*) FROM route_contribution_consents)+(SELECT count(*) FROM route_contribution_limits)+(SELECT count(*) FROM route_contribution_custody)')
  [[ $restored_private == 0 ]]
  restored_api=$(docker run --rm -d --network "$network" \
    -e DATABASE_URL='postgres://byl:smoke-only@postgres:5432/byl_restore?sslmode=disable' \
    -e ROUTE_CONTRIBUTIONS_ENABLED=true -e ROUTE_CONTRIBUTION_CUSTODY_APPROVED=true \
    -e "ROUTE_CONTRIBUTION_SCOPE_KEY=$smoke_scope_key" "$image")
  containers+=("$restored_api")
  ready=false
  for _ in {1..30}; do
    if docker exec "$restored_api" wget -q -O - http://127.0.0.1:8080/api/v1/route-contribution-intake > "$scratch/restored-intake.json"; then ready=true; break; fi
    sleep 1
  done
  [[ $ready == true ]]
  python3 -c 'import json,sys; assert json.load(open(sys.argv[1]))["enabled"] is False' "$scratch/restored-intake.json"
  source_tours=$(docker exec "$postgres" psql -U byl -d byl -Atqc 'SELECT count(*) FROM tours')
  restored_tours=$(docker exec "$postgres" psql -U byl -d byl_restore -Atqc 'SELECT count(*) FROM tours')
  [[ $source_tours -gt 0 && $source_tours == "$restored_tours" ]]
  docker exec -i "$postgres" psql -U byl -d byl -v ON_ERROR_STOP=1 >/dev/null <<'SQL'
UPDATE route_contribution_consents SET created_on=created_on-31,requested_on=requested_on-31,expires_on=expires_on-31;
UPDATE route_contributions SET observation_period=observation_period-31,expires_on=expires_on-31;
UPDATE route_contribution_limits SET expires_on=expires_on-31;
SQL
  docker run --rm --network "$network" -e "DATABASE_URL=$database_url" \
    --entrypoint /contribution-retention "$image"
  remaining_private=$(docker exec "$postgres" psql -U byl -d byl -Atqc 'SELECT (SELECT count(*) FROM route_contributions)+(SELECT count(*) FROM route_contribution_consents)+(SELECT count(*) FROM route_contribution_limits)')
  [[ $remaining_private == 0 ]]
else
  docker exec "$container" wget -q -O - "http://127.0.0.1:$port/release.json" > "$scratch/release.json"
  python3 -c 'import json,sys; assert json.load(open(sys.argv[1])) == {"revision":sys.argv[2]}' "$scratch/release.json" "$revision"
  docker exec "$container" wget -q -O - "http://127.0.0.1:$port/" > "$scratch/index.html"
  grep -q '_app/immutable' "$scratch/index.html"
  docker exec "$container" wget -q -O - "http://127.0.0.1:$port/brand/favicon.svg" > "$scratch/favicon.svg"
  grep -q '<svg' "$scratch/favicon.svg"
fi
echo "$service release image smoke test passed"
