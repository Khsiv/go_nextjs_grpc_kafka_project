# Project Summary

A full-stack web application demonstrating Google OAuth authentication, a REST API in Go, a gRPC microservice, and a Next.js frontend — all orchestrated with Docker Compose backed by PostgreSQL.

## Architecture

```
┌──────────────┐   HTTP/JSON    ┌──────────────┐    gRPC     ┌──────────────┐
│  Next.js UI  │ ─────────────► │  Go REST API │ ──────────► │ gRPC server  │
│  (port 3000) │  cookies/CORS  │  (port 8000) │  MakePost   │ (port 50051) │
└──────────────┘                └──────┬───────┘             └──────┬───────┘
                                       │                            │
                                       ▼                            ▼
                                ┌────────────────────────────────────────┐
                                │        PostgreSQL 13 (port 5433)       │
                                └────────────────────────────────────────┘
```

## Components

### `backend_go/` — REST API (Go 1.25, gorilla/mux)
- Entry point: [main.go](backend_go/main.go); all routing, auth, and handlers live in [controller.go](backend_go/controller/controller.go).
- Implements the full **Google OAuth 2.0 authorization-code flow**: redirect to Google, state validation (in-memory cache), code-for-token exchange, and email verification check.
- Tokens are stored in **HTTP-only cookies** (`google_access_token`, 1 h; `google_refresh_token`, 30 d) plus a frontend-readable `user_email` cookie.
- `googleAuthMiddleware` protects the user CRUD routes by validating the access token against Google's userinfo endpoint on every request.
- Endpoints:
  - Public: `GET /api/go/auth/google`, `GET /api/go/auth/google/callback`, `POST /api/go/auth/refresh`, `POST /api/go/auth/logout` (revokes the token with Google).
  - Protected: user CRUD under `/api/go/users`, `POST /api/go/users/createPost` (delegates to the gRPC service), `GET /api/go/users/{id}/posts`.
- CORS is configured for a specific frontend origin with `Allow-Credentials: true` so cookies work cross-origin.

### `backend_grpc/` — gRPC microservice (Go)
- Single `Greeter` service with one RPC, `MakePost(MeesagePost) → Status`, defined in [backend_service.proto](backend_grpc/proto/backend_service.proto); generated code in `pb/`.
- [server_grpc.go](backend_grpc/server_grpc.go) listens on `:50051` and inserts posts directly into the `posts` table.
- Code generation is containerized via `protoc.dockerfile` / `buf.gen.yaml` (the `protoc_grpc` compose service).

### `frontend_react/` — Next.js 16 frontend (Pages Router, React 19, Tailwind CSS 4, TypeScript)
- [index.tsx](frontend_react/src/pages/index.tsx) — login page; redirects to the backend OAuth endpoint and handles `?error=` / `?login=success` callback params.
- [dashboard.tsx](frontend_react/src/pages/dashboard.tsx) — protected page; validates the session by calling the API and automatically calls `/auth/refresh` on a 401.
- [users/[id]/posts.tsx](frontend_react/src/pages/users/%5Bid%5D/posts.tsx) — per-user posts page.
- Components: `LoginCard`, `GoogleLoginButton`, `ErrorAlert`, `ProtectedContent`, `UserInterface` (user CRUD UI), `CardComponent`.
- Uses `NEXT_PUBLIC_API_URL` (default `http://localhost:8000`) and sends requests with `credentials: 'include'`.

### `migrations/` — golang-migrate SQL migrations
- `users` table: `id`, `name`, `email`, `description`.
- `posts` table: `id`, `users_id` (FK → users), `post_text`.

## Docker Compose services ([compose.yaml](compose.yaml))

| Service | Image / Build | Ports | Notes |
|---|---|---|---|
| `db` | postgres:13 | 5433→5432 | Healthcheck; `pgdata` volume |
| `migrate` | migrate/migrate | — | Runs migrations after db is healthy |
| `grpc_server` | backend_grpc | — | Waits on db + migrate |
| `goapp` | backend_go | 8000 | Waits on db, migrate, grpc_server |
| `nextapp` | frontend_react | 3000 | Depends on goapp |
| `protoc_grpc` | protoc image | — | One-shot proto codegen |

## Authentication design

The app deliberately uses **Google's OAuth tokens directly instead of minting custom JWTs**: Google manages token signing, expiry, and revocation, and the backend validates the access token against Google's userinfo endpoint on every protected request. Tokens never touch `localStorage`/`sessionStorage` — everything lives in cookies:

| Cookie | HttpOnly | Expiry | Purpose |
|---|---|---|---|
| `google_access_token` | Yes | 1 hour | Authenticates API requests |
| `google_refresh_token` | Yes | 30 days | Gets new access tokens via `/auth/refresh` |
| `user_email` | No | 1 hour | Read by the frontend for display |

**Token flow:**
1. **Login** — "Continue with Google" → backend generates a CSRF `state`, redirects to Google → callback validates state, exchanges the code, verifies the email, sets cookies, and redirects to `{FRONTEND_URL}/?login=success&email=...` (errors redirect with `?error=invalid_state|token_exchange_failed|verification_failed`).
2. **API requests** — the browser sends cookies automatically (`credentials: 'include'` on the frontend; CORS must use a specific origin + `Allow-Credentials: true`, never `*`).
3. **Refresh** — on a 401 the frontend calls `POST /api/go/auth/refresh`; the backend uses the refresh token to obtain a new access token from Google and re-sets the cookie, then the frontend retries.
4. **Logout** — `POST /api/go/auth/logout` revokes the token with Google and clears all three cookies.

**Refresh-token gotcha:** Google only issues a refresh token on the *first* consent. The login handler passes `oauth2.AccessTypeOffline` + `oauth2.ApprovalForce` to force a consent prompt so a refresh token is always returned.

## Google OAuth setup (Google Cloud Console)

1. Create/select a project at [console.cloud.google.com](https://console.cloud.google.com/).
2. "APIs & Services" → "Credentials" → "Create Credentials" → "OAuth client ID" → **Web application**.
3. Authorized JavaScript origins: `http://localhost:8000` and `http://localhost:3000`.
4. Authorized redirect URI: `http://localhost:8000/api/go/auth/google/callback` (must match `GOOGLE_REDIRECT_URL` exactly).
5. Copy the Client ID / Client Secret into `.env`.

## Configuration

Environment variables (see `.env`, consumed by compose — never commit it):
- `GOOGLE_CLIENT_ID`, `GOOGLE_CLIENT_SECRET`, `GOOGLE_REDIRECT_URL` (default `http://localhost:8000/api/go/auth/google/callback`)
- `FRONTEND_URL` (default `http://localhost:3000`)
- `DATABASE_URL`, `GRPC_ADDR` (set per-service in compose; local fallbacks are hard-coded for running outside Docker)
- `NEXT_PUBLIC_API_URL` (frontend; default `http://localhost:8000`)

## Running

### With Docker Compose

```bash
# Regenerate gRPC code first if the .proto changed
docker compose run --rm protoc_grpc

docker compose up --build
# Frontend: http://localhost:3000
# API:      http://localhost:8000
# Postgres: localhost:5433 (postgres/postgres)
```

### Locally without Docker

```bash
cd backend_grpc && go run server_grpc.go   # gRPC service on :50051
cd backend_go   && go run main.go          # REST API on :8000
cd frontend_react && npm install && npm run dev   # UI on :3000
```

Both Go services fall back to `postgres://postgres:postgres@localhost:5433/postgres` when `DATABASE_URL` is unset, so the compose `db` service can be reused for local runs.

## Troubleshooting

- **"Invalid state parameter"** — usually caused by refreshing mid-OAuth-flow or stale browser cache; retry the login from the start.
- **"Failed to exchange token"** — check `GOOGLE_CLIENT_ID`/`GOOGLE_CLIENT_SECRET` and that the redirect URI in Google Cloud Console matches `GOOGLE_REDIRECT_URL` exactly.
- **CORS errors** — both apps must be running, `NEXT_PUBLIC_API_URL` must match the backend URL, the backend must send a specific `Access-Control-Allow-Origin` (not `*`) plus `Allow-Credentials: true`, and the frontend must use `credentials: 'include'`.
- **No refresh token received** — Google only sends it on first consent; revoke the app's access in your Google account settings (or clear cookies) and log in again.
- **401 on every request** — verify the `google_access_token` cookie exists and the backend can reach Google's userinfo endpoint.
- **Token refresh test** — delete the `google_access_token` cookie in DevTools and make a request; it should auto-refresh and succeed.

## Production checklist

- [ ] Set `Secure: true` on all cookies (requires HTTPS).
- [ ] Point `FRONTEND_URL`, `GOOGLE_REDIRECT_URL`, and `NEXT_PUBLIC_API_URL` at production domains, with matching Google OAuth production credentials.
- [ ] Add rate limiting and logging/monitoring.
- [ ] Consider persisting refresh tokens in the database to support server-side revocation.

## Known limitations / notes

- OAuth `state` values are kept in an in-memory map — not shared across replicas and never expired.
- Cookies are set with `Secure: false` (dev only); must be `true` behind HTTPS in production.
- Every protected request round-trips to Google's userinfo endpoint for token validation (no local caching).
- Several handlers use `log.Fatal` on DB errors, which kills the process instead of returning a 500.
- The proto message name `MeesagePost` (typo) is baked into the generated code and JSON contract.
