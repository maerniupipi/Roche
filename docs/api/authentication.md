# Authentication API

## 1. Runtime boundary

Authentication is no longer hosted by the management/RAG backend. The deployed request path is:

```text
Browser
  -> API Gateway (internal or external zone)
      -> Auth Service (login, SAML/OIDC callback, refresh, logout)
      -> Application Backend (management, RAG and agent APIs)
```

The same two images are reused in both trust zones:

| Image | Internal deployment | External deployment |
|---|---|---|
| API Gateway | `api-gateway-internal` | `api-gateway-external` |
| Auth Service | `auth-service-internal` | `auth-service-external` |

Each Auth Service instance has its own SAML SP entity ID, ACS URL, certificate and key. Both instances share the platform identity database and JWT signing configuration.

The Application Backend does not register `/api/v1/auth/*`. For a protected `/api/*` request, Gateway first calls the internal token validation endpoint and only then proxies the request to the Application Backend. The backend validates the Bearer token again as defense in depth.

## 2. Authentication modes

| Environment | Password login | Registration | Enterprise SSO |
|---|---:|---:|---|
| Local development | enabled | enabled | Dex OIDC Mock |
| Server development | enabled | enabled | Dex OIDC Mock; SAML can be enabled for integration testing |
| Production | disabled | disabled | PingIdentity SAML 2.0 |

Both SAML and OIDC identities are mapped to local `users` and `sso_identities`. Successful login issues a platform access token and rotating refresh token. Business APIs accept only the platform token:

```http
Authorization: Bearer <platform_access_token>
```

## 3. Public Auth Service endpoints

All browser-facing endpoints are reached through the selected Gateway origin.

| Method | Path | Purpose |
|---|---|---|
| `GET` | `/api/v1/auth/registration/config` | Return password-login and registration capability flags |
| `POST` | `/api/v1/auth/register` | Development-only email account registration |
| `POST` | `/api/v1/auth/login` | Development-only email/password login |
| `GET` | `/api/v1/auth/saml/config` | Return SAML availability and display name |
| `GET` | `/api/v1/auth/saml/url` | Create an SP-initiated SAML request |
| `GET/POST` | `/api/v1/auth/saml/acs` | Consume the PingIdentity SAML response |
| `GET` | `/api/v1/auth/saml/metadata` | Return zone-specific SP metadata XML |
| `GET` | `/api/v1/auth/oidc/config` | Return OIDC availability for development |
| `GET` | `/api/v1/auth/oidc/url` | Create an OIDC authorization request |
| `GET` | `/api/v1/auth/oidc/callback` | Consume the OIDC authorization-code callback |
| `POST` | `/api/v1/auth/refresh` | Rotate refresh token and issue a new access token |

Protected Auth Service endpoints:

| Method | Path | Purpose |
|---|---|---|
| `GET` | `/api/v1/auth/validate` | Validate the current access token |
| `GET` | `/api/v1/auth/me` | Return the current local user |
| `PUT` | `/api/v1/auth/me/preferences` | Update user preferences |
| `POST` | `/api/v1/auth/logout` | Revoke the current token family |
| `POST` | `/api/v1/auth/change-password` | Development password change; rejected in SSO-only production |

### Registration capability response

```json
{
  "success": true,
  "password_login_enabled": true,
  "enabled": true,
  "role_selection_enabled": true,
  "default_role": "viewer",
  "roles": ["viewer", "system_admin"]
}
```

The production values for `password_login_enabled`, `enabled` and `role_selection_enabled` are all `false`.

## 4. PingIdentity SAML flow

1. UI requests `GET /api/v1/auth/saml/config` from its current Gateway.
2. UI requests `/api/v1/auth/saml/url?redirect_uri=<exact-allowed-frontend-uri>`.
3. Auth Service creates a signed AuthnRequest. Signed RelayState contains a nonce, final redirect URI and the AuthnRequest ID; the browser also receives an HttpOnly nonce cookie.
4. Browser is redirected to PingIdentity.
5. PingIdentity authenticates the employee and posts `SAMLResponse` plus `RelayState` to the zone-specific ACS URL.
6. Auth Service validates the IdP signature/certificate, assertion time conditions, audience, recipient, RelayState cookie binding and `InResponseTo` request ID.
7. Auth Service maps the configured NameID/attributes to the stable external identity and upserts `sso_identities` plus the local `users` mapping.
8. If the user is active, Auth Service issues the platform access token and rotating refresh token.
9. Auth Service redirects to the signed, allowlisted frontend URI with `saml_result` in the URL fragment. The frontend consumes the result and removes the fragment from browser history.

Example start request:

```http
GET /api/v1/auth/saml/url?redirect_uri=https%3A%2F%2Fknowledge.example.com%2F
```

PingIdentity must register two SP definitions because the internal and external ACS URLs are different. The IdP identity source may be shared.

IdP-initiated login is disabled by default. Enable it only after a documented security review because it cannot provide the same original-request binding as SP-initiated login.

## 5. OIDC development flow

Dex exercises the existing Authorization Code flow without requiring PingIdentity connectivity. The two server-development callbacks are:

```text
http://<server>:8088/api/v1/auth/oidc/callback
http://<server>:8089/api/v1/auth/oidc/callback
```

The Auth Service validates state, nonce, provider signature and claims, maps `(issuer, subject)` to `sso_identities`, then issues platform tokens. OIDC is forced off by the production startup script.

## 6. Refresh and logout

Refresh request:

```http
POST /api/v1/auth/refresh
Cookie: roche_kap_refresh_token=<refresh-token>
```

The Refresh Token is delivered as an `HttpOnly; SameSite=Lax` cookie scoped to
`/api/v1/auth`; production adds `Secure`. It is never returned in JSON or URL
fragments and is not readable by frontend JavaScript. A successful refresh
invalidates the old refresh token, returns only the new Access Token in JSON,
and rotates the Refresh Token with `Set-Cookie`. The existing `auth_tokens`
persistence format is unchanged.

Client handling rules:

- `401`: access token is missing, invalid or expired; attempt at most one serialized refresh.
- `403`: identity is valid but the operation is forbidden; do not refresh and retry.
- Refresh failure: clear local credentials and start SSO again.

## 7. Internal Gateway validation endpoint

```http
GET /internal/v1/auth/validate
Authorization: Bearer <platform_access_token>
X-Auth-Service-Secret: <gateway-auth-shared-secret>
```

This endpoint is not exposed by Gateway. Nginx calls it through an internal `auth_request` location. A successful response is `204` with trusted identity headers. Gateway overwrites client-supplied identity headers before forwarding the original request.

## 8. Required production controls

- `AUTH_PASSWORD_LOGIN_ENABLE=false`
- `AUTH_REGISTRATION_ENABLE=false`
- `OIDC_AUTH_ENABLE=false`
- `SAML_AUTH_ENABLE=true`
- Exact `AUTH_*_ALLOWED_REDIRECT_URIS`; wildcard redirects are not accepted.
- HTTPS IdP metadata and both HTTPS ACS URLs.
- Distinct, stable certificate/key mounts for internal and external SPs.
- Separate `AUTH_INTERNAL_SERVICE_SECRET` and `AUTH_EXTERNAL_SERVICE_SECRET` values of at least 48 characters, injected as secrets.
- `AUTH_REFRESH_COOKIE_SECURE=true`.
- `SAML_AUTH_ALLOW_EPHEMERAL_CERT=false`.
- No direct public ports for App, Auth Service or Frontend; only Gateway/Ingress is exposed.
