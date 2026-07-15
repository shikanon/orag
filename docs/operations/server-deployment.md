# Reference server deployment

This guide describes the reference deployment used for the ORAG community
demo. It keeps credentials and runtime state on the server and uses the
published GHCR images, so the procedure is reproducible without cloning the
repository on the host.

The current reference host is `root@8.134.24.116`. Use a dedicated hostname
such as `orag.tensorbytes.com`; do not repoint the existing
`www.tensorbytes.com` site unless its current content has been migrated and
the DNS change has been approved.

## 1. DNS and host prerequisites

Create an `A` record in the `tensorbytes.com` DNS zone:

```text
orag.tensorbytes.com -> 8.134.24.116
```

On the host, confirm that Docker and Compose are available. The reference
host currently provides the legacy `docker-compose` command. The commands
below use it explicitly; installing the Docker Compose v2 plugin and changing
the command to `docker compose` is also supported.

```bash
ssh root@8.134.24.116
docker --version
docker-compose version
```

Install or renew TLS with the host's existing certificate automation before
exposing the service publicly. Do not put the SSH key path, API keys, database
passwords, or JWT secret in this repository.

## 2. Prepare runtime state

Create a private deployment directory and a server-only environment file:

```bash
install -d -m 0750 /opt/orag
cd /opt/orag
umask 077
${EDITOR:-vi} .env
```

Start from the repository example and replace every demo value. At minimum,
set:

```dotenv
ORAG_VERSION=v0.1.0-beta.2
ORAG_API_IMAGE=ghcr.io/shikanon/orag-api:v0.1.0-beta.2
ORAG_CONSOLE_IMAGE=ghcr.io/shikanon/orag-console:v0.1.0-beta.2
PUBLIC_BASE_URL=https://orag.tensorbytes.com
POSTGRES_IMAGE=postgres:16-alpine
QDRANT_IMAGE=qdrant/qdrant:v1.11.5
JWT_SECRET=<random-long-secret>
ADMIN_DEFAULT_USERNAME=<bootstrap-admin>
ADMIN_DEFAULT_PASSWORD=<random-long-password>
DOCKER_DATABASE_URL=postgres://orag:<database-password>@postgres:5432/orag?sslmode=disable
LLM_CHAT_PROVIDER=<production-provider>
LLM_EMBEDDING_PROVIDER=<production-provider>
LLM_RERANK_PROVIDER=<production-provider>
LLM_MULTIMODAL_PROVIDER=<production-provider>
ALLOW_DETERMINISTIC_MOCK=false
```

Inject provider and storage credentials through this file or a secret manager.
Never commit it, copy it into an image, or print it in CI logs.

If the host cannot reach Docker Hub, replace `POSTGRES_IMAGE` and
`QDRANT_IMAGE` with organization-approved mirrors before pulling. The API and
Console images remain pinned to GHCR release tags (and should be recorded by
digest after the pull).

Download the exact Compose file from the release tag and verify its checksum
through the GitHub release before starting the stack:

```bash
curl --fail --location --output docker-compose.yml \
  https://raw.githubusercontent.com/shikanon/orag/v0.1.0-beta.2/deployments/docker-compose.yml
```

## 3. Start and verify the stack

The Compose file starts PostgreSQL, Qdrant, a one-shot migration, API and
Console. The `demo` profile is intentionally not enabled in production.

```bash
docker-compose --env-file .env pull postgres qdrant orag-api orag-console
docker-compose --env-file .env up -d postgres qdrant
docker-compose --env-file .env up migrate
docker-compose --env-file .env up -d orag-api orag-console
docker-compose --env-file .env ps
curl --fail https://orag.tensorbytes.com/healthz
curl --fail https://orag.tensorbytes.com/readyz
```

For a disposable no-key validation only, use the explicit demo profile from a
separate directory and never reuse production volumes:

```bash
docker-compose -f docker-compose.yml --profile demo up --build --wait
docker-compose -f docker-compose.yml --profile demo down -v
```

## 4. Reverse proxy

Terminate TLS at the existing Nginx installation and proxy the Console and
API separately. The API must receive `/healthz`, `/readyz`, `/docs`, `/v1/*`,
and SSE requests without buffering; the Console serves the browser shell.

```nginx
server {
    listen 443 ssl http2;
    server_name orag.tensorbytes.com;

    # certificate and key are provisioned outside this repository
    ssl_certificate /etc/letsencrypt/live/orag.tensorbytes.com/fullchain.pem;
    ssl_certificate_key /etc/letsencrypt/live/orag.tensorbytes.com/privkey.pem;

    location /api/ {
        proxy_pass http://127.0.0.1:8080/;
        proxy_http_version 1.1;
        proxy_buffering off;
        proxy_read_timeout 300s;
        proxy_set_header Host $host;
        proxy_set_header X-Forwarded-Proto $scheme;
        proxy_set_header X-Forwarded-For $proxy_add_x_forwarded_for;
    }

    location / {
        proxy_pass http://127.0.0.1:3000;
        proxy_http_version 1.1;
        proxy_set_header Host $host;
        proxy_set_header X-Forwarded-Proto $scheme;
        proxy_set_header X-Forwarded-For $proxy_add_x_forwarded_for;
    }
}
```

If the Console is configured with an API base URL other than `/api`, keep the
proxy path and Console configuration aligned. Validate before reload:

```bash
nginx -t && systemctl reload nginx
```

## 5. Operations and rollback

- Back up the PostgreSQL volume and Qdrant storage before every version change.
- Record the image digests shown by `docker-compose images` in the deployment log.
- Run `/readyz`, a cited query, and a trace lookup after each upgrade.
- Roll back by changing `ORAG_*_IMAGE` and `ORAG_VERSION` to the previous
  immutable release, then recreate only API and Console. Never move or reuse
  a published Git tag.
- Keep `ALLOW_DETERMINISTIC_MOCK=false` and replace bootstrap credentials before
  inviting community users.

For troubleshooting, see [`troubleshooting.md`](troubleshooting.md),
[`../getting-started/api-smoke.md`](../getting-started/api-smoke.md), and the
release notes for the exact version being deployed.
