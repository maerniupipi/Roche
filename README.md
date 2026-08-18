# Roche Knowledge Agent Platform

Roche Knowledge Agent Platform is an enterprise knowledge base and intelligent agent platform for internal document understanding, semantic retrieval, knowledge governance, and assisted question answering.

## Architecture

- `frontend`: Vue 3 management and conversation interface
- `cmd/server` and `internal`: Go API service and processing pipeline
- `docreader`: Python gRPC document parsing service
- PostgreSQL: business data and metadata
- Redis: task queue and runtime state
- Milvus or another configured vector engine: embedding storage and retrieval
- Neo4j: optional knowledge graph storage

## Local Source Development

Create the only local environment file, then start the complete development
stack:

```powershell
Copy-Item .env.local.example .env.local
& "C:\Program Files\Git\bin\bash.exe" "F:/RocheKAP/scripts/dev-all.sh"
```

This runs Go and Vite on the host and starts PostgreSQL, Redis, DocReader,
Milvus, Neo4j, Langfuse, and Dex in Docker. Knowledge files use
`.local-data/files` by default; business MinIO is opt-in with `--minio`.

Stop it without deleting data:

```powershell
& "C:\Program Files\Git\bin\bash.exe" "F:/RocheKAP/scripts/dev-all.sh" stop
```

The local infrastructure is defined only in `docker-compose.local.yml`.

Start infrastructure:

```bash
make dev-start
```

Start the Go backend and Vue frontend in separate terminals:

```bash
make dev-app
make dev-frontend
```

See [Local development](docs/development/local-development.md) for environment
setup and troubleshooting.

## Server Source Development

Use this mode when developers edit checked-out source on a Linux server. The
application images are rebuilt from that working tree, while named volumes keep
the development data. Email registration, Dex Mock SSO, and Workday Mock are
enabled. This mode builds on container startup; it is not a hot-reload server.

```bash
cp .env.server-dev.example .env.server-dev
# Set SERVER_DEV_PUBLIC_URL in .env.server-dev first.
make server-dev-up
```

Update the Git worktree and recreate the development containers:

```bash
make server-dev-update
```

## Production Deployment

Production uses immutable CI images, real PingIdentity/OIDC and the HTTP
Workday adapter. It does not mount source code, enable development role
selection, or start Dex. The production script refuses placeholder secrets,
`latest` application images, mock Workday, or an enabled registration page.

```bash
cp .env.production.example .env.production
# Replace every CHANGE_ME value and set fixed image tags.
make production-up
```

See [Three deployment modes](docs/development/deployment-modes.md) for the
complete startup, shutdown, update, security, and data-preservation procedures.

## Enterprise Access

The platform supports email/password accounts, optional enterprise OIDC, department-scoped administration, and explicit knowledge base or agent access grants. Production deployments should disable registration role selection and connect OIDC to the customer identity provider when required.

## Legal

Required software license and third-party notices are retained in [LICENSE](LICENSE).
