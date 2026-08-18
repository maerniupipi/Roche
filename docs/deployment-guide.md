# RocheKAP Deployment Guide

This page is the stable entry point for deployment documentation. The former
three-image deployment, direct public App exposure, and `latest` image tags are
no longer part of the supported architecture.

## Supported Modes

Use [development/deployment-modes.md](development/deployment-modes.md) for the
complete commands and environment variables for:

1. Local source development.
2. Server source-mounted development.
3. Production deployment with immutable images.

## Authentication Topology

Production authentication is deployed as two image types and four runtime
instances:

| Image type | Internal instance | External instance |
|---|---|---|
| API Gateway | `api-gateway-internal` | `api-gateway-external` |
| Auth Service | `auth-service-internal` | `auth-service-external` |

The browser enters through the appropriate API Gateway. The Gateway validates
the platform JWT and routes unauthenticated login traffic to the matching Auth
Service. PingIdentity SAML integration, token issuance, refresh, and logout are
owned by Auth Service; the Management/DeepRAG backend does not expose login
endpoints.

See [architecture/auth-service-api-gateway.md](architecture/auth-service-api-gateway.md)
for trust boundaries and request flows, and
[api/authentication.md](api/authentication.md) for endpoint details.

## Production Images

Production uses five immutable application images:

- App backend
- Frontend
- DocReader
- Auth Service
- API Gateway

Use a fixed version tag or digest for every image. Do not deploy `latest`.
Stateful infrastructure such as PostgreSQL, Redis, Milvus, MinIO, Neo4j, and
Langfuse must use persistent volumes and must not be recreated as part of an
application-only rollout.

## Public Exposure

Only the internal or external API Gateway should be exposed to its intended
network zone. App, Frontend, Auth Service, DocReader, and data services remain
on private container networks. Internal and external SAML entity IDs, ACS URLs,
certificates, keys, and redirect allowlists are configured independently.
