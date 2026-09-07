# CLAUDE.md — LogStream

Context file for AI assistants working in this repo. Keep it current when architecture changes.

## What this is

LogStream is a real-time log aggregation pipeline: agents/collectors push logs over HTTP → a Go
API batches them to PostgreSQL → a React dashboard queries history and streams a live tail over
WebSocket. An alert engine matches log messages against user-defined regex rules.

Recent architectural shift (see git log): moved from a push-based CLI collector model toward a
**pull-based model using a Vector agent** that tails files and POSTs to `/ingest`. The old
`logstream collect` stdin pipe still exists.

## Layout

```
cmd/logstream/main.go        Cobra CLI: `serve` (API server) and `collect` (stdin pipe agent)
internal/
  api/
    router.go                chi routes, all HTTP handlers, JWT dispenser for WS; takes api.Config
    middleware.go            RequestID, Logger (slog), CORS(origins), Recoverer, RateLimit(trustedProxies), APIKeyAuth
    security.go              api.Config + helpers: clientIP/XFF, origin allowlist, WS secret derivation, CIDR parsing
    websocket.go             Hub + Client; Run goroutine solely owns the clients map; non-blocking broadcast; HS256-pinned JWT + origin check
  logging/logging.go         slog setup (LOG_FORMAT/LOG_LEVEL) + request-id context handler
  collector/
    http.go                  POST /ingest: content-type check, idempotency LRU (10m TTL), 10MB/1000-batch/16KB-metadata caps
    batcher.go               Worker pool (NumCPU workers) → WAL segment → DB insert → alerts + broadcast (no shared lock)
    wal.go                   Segmented WAL: one fsync'd file per batch under <WAL_PATH-base>.wal.d/, deleted only after its own insert; replayed on startup
    stdin.go                 `collect` command: multi-line-aware stdin reader → POST /ingest
  storage/
    postgres.go              pgxpool setup, InsertBatch (transactional pgx.Batch + RETURNING id)
    partitions.go            EnsurePartitions / RunPartitionMaintenance — weekly partition provisioning
    queries.go               Search (escaped ILIKE + trigram), Stats, Services, Alert rule CRUD, ListAlerts
  alerts/engine.go           Compiled-regex cache, per-rule cooldown map, Check() runs per batch
  models/log.go              LogEntry, AlertRule, StatsResult, AlertWithContext
  parser/
    parser.go                LogParser interface (Parse, ParseBatch)
    json.go                  Canonical JSON parser (used by the server /ingest handler)
    text.go                  Multi-format text parser (logstream/uvicorn/postgres/go-http/nginx/fallback)
    docker.go                Docker envelope + flexible-JSON (msg/time aliases) parser, ANSI stripping
frontend/                    React 19 + Vite + react-router + recharts dashboard
  default.conf.template      nginx: SPA + proxy /api,/ws (injects X-API-Key), /health; rendered via envsubst
  src/config/api.js          same-origin API_BASE/WS_BASE; no API key in prod bundle (nginx injects it)
  src/pages/                 Dashboard, Logs, LiveTail, Rules, Alerts
vector_config/vector.yaml    Vector agent: tail /var/log/app/*.log → parse_json → POST /ingest
init.sql                     Schema: partitioned logs table, trigram + GIN indexes, alert_rules, alerts
docker-compose.yml           postgres + logstream-api + logstream-frontend (nginx) + vector-agent
scripts/                     loadtest.go (firehose), ingest_ml_data.py (LogHub datasets), dogfood.sh
```

## Runtime / config

- Go 1.26, module `github.com/logstream`. Postgres 16 (needs `pg_trgm`).
- Server env: `DATABASE_URL`, `API_KEY`, `INGEST_KEY` (defaults to `API_KEY`), `WAL_PATH`,
  `INGEST_ENABLED` (`"false"` = read-only), `ALLOW_INSECURE_DEFAULTS` (`"true"` lets the server
  boot with `dev-key` / default DB password), `WS_JWT_SECRET` (derived from `API_KEY` if unset),
  `ALLOWED_ORIGINS` (CORS + WS origin allowlist, comma list, `*` for dev), `TRUSTED_PROXIES`
  (CIDR list whose `X-Forwarded-For` is trusted; default = loopback + RFC1918),
  `LOG_FORMAT` (`json`|`text`), `LOG_LEVEL` (`debug|info|warn|error`), `PORT` (used when `--port` absent).
- Flags override env: `--port --db-url --api-key --wal-path`.
- Logging is `log/slog` (`internal/logging`); every request gets an `X-Request-ID` that also
  tags its log lines. The `collect` CLI keeps plain stdout output.
- Config is assembled into `api.Config` in `main.go` and passed to `api.NewRouter`.
- Frontend build env: `VITE_API_URL`, `VITE_WS_URL`, `VITE_API_KEY` (baked into static bundle).
- Local ports: API `8090`, frontend `5173`, postgres host `5433`.
- Prometheus metrics at `/metrics`; `/health` pings the DB.

## Request flow

1. `POST /ingest` (auth: `INGEST_KEY` via `X-API-Key`, rate limited) → `HTTPHandler.ServeHTTP`
   → `JSONParser.ParseBatch` → `Batcher.Send` (drops at 80% channel capacity with backpressure).
2. Batcher worker collects up to 100 entries or 2s tick → `flush()` (no shared lock):
   `wal.AppendSegment` → `store.InsertBatch` (single `pgx.Tx`, RETURNING id populates entries) →
   `wal.RemoveSegment` (only on insert success) → `engine.Check(batch)` →
   `hub.Broadcast(batch)` (non-blocking hand-off to the Hub.Run goroutine).
3. `GET /api/*` requires `X-API-Key` header + passes `RateLimit`. Search uses
   `message ILIKE $1 ESCAPE '\'` (`likePattern` escapes `\ % _`, then spaces → `%`) backed by
   `idx_logs_message_trgm`. Unfiltered search is bounded to the last 30 days; `GET /api/logs/{id}`
   takes an optional `?ts=` RFC3339 hint for partition pruning.
4. Live tail: `GET /api/ws-token` issues a 60s HS256 JWT signed with the derived WS secret →
   client opens `GET /ws/tail?service=…&level=…` with subprotocols `['logstream','auth.<jwt>']`
   (token in `Sec-WebSocket-Protocol`, not the URL; `?token=` still accepted) → `Hub.ServeWS`
   checks Origin then validates the JWT (HS256 only, expiry required) → registers client.

## Conventions / gotchas

- Canonical levels: DEBUG, INFO, WARN, ERROR, FATAL (`normaliseLevel` maps WARNING/CRITICAL/PANIC/etc.).
- Handlers return `[]` not `null` for empty slices (frontend depends on it).
- `logs` RANGE-partitioned on `timestamp`; `storage.EnsurePartitions` (startup + 6h goroutine)
  keeps weekly partitions provisioned for a rolling window. `logs_default` is the catch-all.
- Alert dedup relies on `ON CONFLICT (rule_id)`; `init.sql` has `idx_alerts_rule_id` UNIQUE for it.
- Alert rule limits (`internal/api/router.go` `validateRule`): name ≤200, pattern ≤500, cooldown
  0–10080 min, ≤500 rules total.
- `engine.Check` snapshots compiled rules under a brief lock, then matches lock-free.
- WAL is segmented: `<WAL_PATH-base>.wal.d/seg-*.json`, one fsync'd file per batch; `Replay`
  quarantines `*.corrupt` (bad JSON) / `*.failed` (won't insert) and cleans stray `*.tmp`.
- Search is trigram/ILIKE on `message` only (the old tsvector column was removed).
- Tests: `internal/api/{security,rules}_test.go`, `internal/collector/{wal,http}_test.go`,
  `internal/storage/queries_test.go`, `internal/parser/parser_test.go`. Run `go test ./...`
  (add `-race` for api/collector).
- `docker-compose` needs a `.env` (copy `.env.example`); postgres mounts `init.sql`.

## Build / run

```bash
docker compose up --build            # full stack
go build -o logstream ./cmd/logstream # server binary
go run ./scripts/loadtest.go          # 10k-log firehose against :8090
```

See `ISSUES.md` for the current catalogue of bugs and security vulnerabilities.
