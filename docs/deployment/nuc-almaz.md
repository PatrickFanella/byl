# NUC / Almaz deployment

The Go API, PostgreSQL, MinIO (media), and Valhalla (routing) run on the NUC.
Cloudflare and Caddy stay on Almaz and reverse-proxy the public hostname to the NUC.

## Services

- `postgres` stays on the private Docker network only.
- `api` listens on `:8080` inside Docker and publishes `8080` on the NUC for Almaz to reach. It also runs the background job worker.
- `minio` stores uploaded media and narration audio. Publish port `9000` to the LAN so Caddy can serve the bucket, or front it with Cloudflare R2 later by changing the `S3_*` variables.
- `valhalla` computes walking and cycling routes from an Illinois OpenStreetMap extract. Its first start downloads the extract and builds tiles; budget 20 to 60 minutes and several GB of disk. Route jobs that run before it is ready fail after three attempts; retry them from the admin API once Valhalla reports it is serving tiles, or publish tours after that point.

## Environment

Create a private `.env` next to `docker-compose.nuc.yml`:

```bash
POSTGRES_USER=museum
POSTGRES_PASSWORD=change-me
DATABASE_URL=postgres://museum:change-me@postgres:5432/invisible_museum?sslmode=disable
PUBLIC_BASE_URL=https://museum-api.subcult.tv
API_PUBLISHED_ADDR=8080

ADMIN_BOOTSTRAP_EMAILS=you@example.com
APPLE_CLIENT_IDS=com.rork.InvisibleMuseum
GOOGLE_CLIENT_IDS=xxxx.apps.googleusercontent.com

ROUTING_PROVIDER=valhalla
NARRATION_PROVIDER=none
MEDIA_STORE=s3
S3_ACCESS_KEY=museum
S3_SECRET_KEY=change-me
S3_BUCKET=museum-media
MEDIA_PUBLIC_BASE_URL=https://museum-media.subcult.tv/museum-media
```

To enable drafting, add one of:

```bash
LLM_PROVIDER=anthropic
LLM_API_KEY=sk-ant-...
LLM_MODEL=claude-opus-5-5
```

```bash
LLM_PROVIDER=openai
LLM_BASE_URL=http://ollama.lan:11434   # or https://openrouter.ai/api, or leave empty for OpenAI
LLM_API_KEY=...
LLM_MODEL=llama3.1:70b
```

and list the local archive pages the `url` source should read:

```bash
LOCAL_ARCHIVE_URLS=https://www.dghistory.org/blodgett-house,https://www.dgparks.org/downers-grove-museum
```

`DEV_AUTH_ENABLED` is hard-coded to `false` in the deployment compose file. Never enable it outside local development: it lets anyone sign in as any email.

Do not publish the Postgres port. The deployment compose file does not expose it.

## First boot

```bash
make nuc-up
```

Then create the media bucket once and make it publicly readable:

```bash
docker compose -f backend/docker-compose.nuc.yml exec minio sh -c \
  'mc alias set local http://localhost:9000 "$MINIO_ROOT_USER" "$MINIO_ROOT_PASSWORD" && mc mb -p local/museum-media && mc anonymous set download local/museum-media'
```

Watch Valhalla build its tiles:

```bash
docker compose -f backend/docker-compose.nuc.yml logs -f valhalla
```

When it reports that it is serving tiles, retry any failed route jobs from the admin API (`POST /api/v1/admin/jobs/{id}/retry`), or publish tours after that point.

Sign in once with the bootstrap email through a client to receive the admin role.

## Caddy / Cloudflare

Sample Caddyfile: `backend/deploy/Caddyfile.almaz`

```caddy
museum-api.example.com {
	encode zstd gzip
	reverse_proxy http://nuc.lan:8080
}

museum-media.example.com {
	encode zstd gzip
	reverse_proxy http://nuc.lan:9000
}
```

Replace `nuc.lan` with the NUC hostname or LAN IP reachable from Almaz. Point the Cloudflare DNS/tunnel routes at Almaz, not directly at the NUC.

## Make commands

From repo root:

```bash
make nuc-up
make nuc-logs
make nuc-smoke
make nuc-rollback
make nuc-down
```

`make nuc-smoke` checks `GET /healthz` and `GET /api/v1/cities`.

## Rollback

Rollback one migration:

```bash
make nuc-rollback
```

If the schema needs a full reset, stop the stack, back up data, then run Goose down/up manually. The first migration of this release replaces the previous schema entirely.

## Smoke checks

```bash
curl http://localhost:8080/healthz
curl http://localhost:8080/api/v1/cities
curl http://localhost:8080/api/v1/tours/downtown-memory-loop
```

From Almaz, verify the upstream before editing the public route:

```bash
curl http://nuc.lan:8080/healthz
```

When going through Caddy/Cloudflare, use the public hostname instead of localhost.
