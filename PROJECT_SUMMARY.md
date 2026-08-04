# Project Summary

A full-stack web application demonstrating Google OAuth authentication, a REST API in Go, a gRPC microservice, an event-driven Kafka microservice, and a Next.js frontend — all orchestrated with Docker Compose backed by PostgreSQL.

## Architecture

```
┌──────────────┐   HTTP/JSON    ┌──────────────┐    gRPC     ┌──────────────┐
│  Next.js UI  │ ─────────────► │  Go REST API │ ──────────► │ gRPC server  │
│  (port 3000) │  cookies/CORS  │  (port 8000) │  MakePost   │ (port 50051) │
└──────▲───────┘                └──────┬───────┘             └──────┬───────┘
       │                               │                            │
       │  SSE           post-stats-requests                         │
       │  (live)                       │                            │
       │                               ▼                            │
       │                     ┌───────────────────┐                  │
       │                     │  Kafka (KRaft)    │                  │
       │                     │  :9092 / :29092   │                  │
       │                     └─────────┬─────────┘                  │
       │                               │                            │
       │                    post-stats-requests                     │
       │                               ▼                            │
       │                     ┌───────────────────┐                  │
       └─────────────────────│   stats_service   │                  │
          post-stats-results └─────────┬─────────┘                  │
          (API consumes, pushes)       │                            │
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
  - Protected (character counts): `GET /api/go/users/stats`, `POST /api/go/users/{id}/stats/recalculate`, `GET /api/go/users/{id}/stats`, `GET /api/go/users/{id}/stats/stream` (SSE).
- Route registration order matters — `/api/go/users/stats` must stay ahead of `/api/go/users/{id}`; [controller_test.go](backend_go/controller/controller_test.go) locks this in.
- CORS is configured for a specific frontend origin with `Allow-Credentials: true` so cookies work cross-origin.

### `stats_service/` — Kafka microservice (Go)
- Consumes `post-stats-requests`, sums `LENGTH(post_text)` across the user's posts, upserts `user_post_stats`, then produces to `post-stats-results`.
- [calculator.go](stats_service/calculator.go) holds the SQL; [main.go](stats_service/main.go) holds the consume/produce loop, topic creation with retry, and graceful shutdown.
- Offsets are committed only after the row is written, so a crash replays the request rather than dropping it.
- Uses `segmentio/kafka-go` (pure Go, no cgo — builds on the same alpine image as the other services).

### `events/` — shared Kafka contract (Go module)
- One definition of the topic names and the `StatsRequest` / `StatsResult` message shapes, imported by both `backend_go` and `stats_service` via a `replace` directive.

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
- `user_post_stats` table: `users_id` (PK, FK → users `ON DELETE CASCADE`), `total_characters`, `posts_count`, `status`, `request_id`, `error_message`, `requested_at`, `calculated_at`.

## Docker Compose services ([compose.yaml](compose.yaml))

| Service | Image / Build | Ports | Notes |
|---|---|---|---|
| `db` | postgres:13 | 5433→5432 | Healthcheck; `pgdata` volume |
| `kafka` | apache/kafka:3.9.0 | 29092 | Single-node KRaft (no Zookeeper); healthcheck; `kafkadata` volume |
| `migrate` | migrate/migrate | — | Runs migrations after db is healthy |
| `grpc_server` | backend_grpc | — | Waits on db + migrate |
| `stats_service` | stats_service | — | Waits on db, migrate, kafka |
| `goapp` | backend_go | 8000 | Waits on db, migrate, grpc_server, kafka |
| `nextapp` | frontend_react | 3000 | Depends on goapp |
| `protoc_grpc` | protoc image | — | One-shot proto codegen |

Kafka listeners: containers connect via `kafka:9092`; `localhost:29092` is published so the
Go services can also be run outside Docker against the same broker. Do not write `0.0.0.0`
into `KAFKA_LISTENERS` — Kafka derives the controller's advertised address from it and
rejects that meta-address at startup.

## Character-count feature (Kafka + SSE)

Pressing **Count Characters** on a user card totals the characters across all of that
user's posts without blocking the request:

1. The browser opens the SSE stream **first**, and only publishes once it is open — so the
   result can never arrive before anything is listening for it.
2. `POST /api/go/users/{id}/stats/recalculate` marks the row `pending` and publishes a
   `StatsRequest` to `post-stats-requests`, keyed by user id. It returns `202` immediately.
3. `stats_service` consumes it, counts, upserts `user_post_stats`, and publishes a
   `StatsResult` to `post-stats-results`.
4. `goapp` consumes that topic and fans the result out to any SSE connection watching that
   user; the label next to the name updates live.

Notes:
- The API's result consumer uses a **unique consumer group per process**, because every
  replica must see every result — it cannot know which one holds the browser's connection.
- The stream sends the stored row on connect as a separate `snapshot` event, distinct from
  the live `stats` event, so the client can render the last known value without mistaking
  it for the answer it is waiting on.
- `LENGTH()` counts characters, not bytes — an emoji counts as 1, not 4.
- Set `STATS_CALC_DELAY_MS=2000` on `stats_service` to make the "Calculating..." state
  visible while demoing.

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
cd backend_grpc  && go run server_grpc.go  # gRPC service on :50051
cd stats_service && go run .               # Kafka consumer
cd backend_go    && go run main.go         # REST API on :8000
cd frontend_react && npm install && npm run dev   # UI on :3000
```

All three Go services fall back to `postgres://postgres:postgres@localhost:5433/postgres` when `DATABASE_URL` is unset, and to `localhost:29092` when `KAFKA_BROKERS` is unset, so the compose `db` and `kafka` services can be reused for local runs.

### Tests

```bash
cd backend_go && go test ./...
```

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
- `initOAuth` prints `GOOGLE_CLIENT_SECRET` to stdout, so the secret ends up in container logs.
- `GetRouter` calls `http.ListenAndServe` itself and never returns, so the call in `main.go` and every `defer` there is dead code.
- Kafka runs as a single broker with replication factor 1 — fine for dev, no redundancy.
- The API's per-process consumer group leaves an empty group behind on each restart; Kafka expires them, but they are visible in `kafka-consumer-groups.sh --list` in the meantime.
