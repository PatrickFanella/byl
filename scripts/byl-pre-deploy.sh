#!/usr/bin/env bash
# Installed by the operator, never taken from an uploaded image.
set -euo pipefail
umask 077
root=/srv/apps/projects/byl
revision=$(git ls-remote --exit-code https://git.subcult.tv/subculture-collective/invisible-museum.git refs/heads/main | cut -f1)
[[ $revision =~ ^[0-9a-f]{40}$ ]]
image="fleet-deploy/byl/api:$revision"
manifest=$(docker run --rm --network none --entrypoint sh "$image" -c 'sha256sum /migrations/*.sql')
[[ $manifest == "$(cat "$root/migrations.sha256")" ]] || {
  echo 'Schema changes require an operator-reviewed migration before deployment.' >&2
  exit 1
}
version=$(docker exec byl-postgres-1 psql -U byl -d byl -Atqc 'SELECT max(version_id) FROM goose_db_version WHERE is_applied')
[[ $version == 13 ]]
backup="$root/backups/$(date -u +%Y%m%dT%H%M%S)-$revision"
mkdir -p "$backup"
docker exec byl-postgres-1 pg_dump -U byl -d byl -Fc > "$backup/database.dump"
test -s "$backup/database.dump"
docker exec -i byl-postgres-1 pg_restore --list < "$backup/database.dump" > "$backup/database.list"
docker exec byl-api-1 tar -C /data/media -czf - . > "$backup/media.tar.gz"
tar -tzf "$backup/media.tar.gz" >/dev/null
cp "$root/migrations.sha256" "$backup/"
echo 'Schema admission and database/media backup passed.'
