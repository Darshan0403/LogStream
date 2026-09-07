# LogStream — Issues, Bugs & Vulnerabilities

Audit date: 2026-09-07. Severity is relative to a service that could be exposed on a network.

---

## Critical

> **Status: C1–C4 fixed** (see commit / working tree). Details inline below.

### C1. `POST /ingest` has no authentication and no rate limiting — FIXED
`internal/api/router.go` mounts `/ingest` as a **public** route. `APIKeyAuth` and `RateLimit`
are only applied inside the `r.Route("/api", …)` group. The rate-limit comment even says
"protects read endpoints only".

Impact: any unauthenticated client can inject unlimited arbitrary log records (forged services,
forged levels, forged timestamps, 10 MB per request, 1000 logs per batch), exhausting disk / DB
storage and poisoning dashboards and alerts. The only mitigation is setting `INGEST_ENABLED=false`,
which disables ingestion entirely and defaults to `true`.

Fix: require an ingestion key (separate from the read API key), apply a real rate limiter, and/or
require mTLS between the Vector agent and the API.

### C2. Alert persistence is broken — `ON CONFLICT (rule_id)` has no matching constraint — FIXED
`internal/storage/queries.go` `CreateAlert` does
`INSERT INTO alerts … ON CONFLICT (rule_id) DO UPDATE …`, but `init.sql` defines `alerts` with
only `id UUID PRIMARY KEY` and no unique/exclusion constraint on `rule_id`.

Impact: Postgres raises `there is no unique or exclusion constraint matching the ON CONFLICT
specification` on **every** alert fire. No alert is ever recorded; `engine.Check` logs the error
and the cooldown timestamp is not updated, so the same failing insert is retried on every batch.

Fix: add `CREATE UNIQUE INDEX ON alerts (rule_id);` (matches the intended one-row-per-rule dedup
model) or rework the dedup logic.

### C3. Hardcoded default credentials shipped everywhere — FIXED
`API_KEY: dev-key` and Postgres `password` appear in `docker-compose.yml`, `vector_config/vector.yaml`,
`scripts/dogfood.sh`, `scripts/ingest_ml_data.py`, `scripts/loadtest.go`, `frontend/src/config/api.js`,
and as the in-code fallback in `cmd/logstream/main.go` (`apiKey = "dev-key"`) and `middleware.go`.

Impact: if deployed without overriding env vars the API and DB are effectively unauthenticated.
The Go server silently starts with `dev-key` rather than refusing to boot.

Fix: fail startup when `API_KEY`/`DATABASE_URL` are unset in non-dev mode; remove committed secrets;
document required env.

### C4. The read API key is baked into the browser bundle — FIXED
`frontend/src/config/api.js` reads `VITE_API_KEY` and `frontend/Dockerfile` builds the static site.
Any value ends up in publicly served JS. `apiFetch` sends it on every call, including
`POST /api/rules`, `PUT /api/rules/{id}`, `DELETE /api/rules/{id}`.

Impact: every visitor to the dashboard obtains full API credentials and can create/modify/delete
alert rules and read all logs. The API key is also the JWT signing secret (see H1), so visitors
can mint arbitrary WebSocket tokens.

Fix: the browser must talk to a session-authenticated backend-for-frontend; never ship a shared
secret to the client. At minimum, split read-only vs. mutating scopes.

---

## High

> **Status: H1–H6 fixed.** See fix notes at the bottom.

### H1. WebSocket JWTs are signed with the API key and the signing method is not pinned — FIXED
`router.go` `wsTokenHandler`: `token.SignedString([]byte(a.apiKey))`.
`websocket.go` `ServeWS`: `jwt.Parse(tokenStr, func(t) { return []byte(jwtSecret), nil })` with no
`jwt.WithValidMethods([]string{"HS256"})` and no claims validation beyond expiry.

Impact: (a) anyone who knows the API key (which is public, see C4) can forge tokens; (b) reusing the
API key as an HMAC secret couples two unrelated trust domains — rotating the API key silently
invalidates all tokens and vice versa; (c) not pinning the algorithm is a latent
algorithm-confusion footgun.

Fix: dedicated random signing secret, `WithValidMethods`, verify `exp`/`iat`/`nbf` explicitly.

### H2. Rate limiter is defeated by the reverse proxy and mis-keys clients — FIXED
`middleware.go` `RateLimit` keys on `r.RemoteAddr` (minus port). Behind `nginx.conf`
(`proxy_pass`), every request arrives from the nginx container IP. nginx sets `X-Forwarded-For`
but the middleware ignores it.

Impact: all dashboard users share a single 100 rps / 200 burst bucket (accidental global DoS), while
a direct attacker hitting the Go port bypasses nginx and gets a per-IP bucket. `/ingest` is not
rate-limited at all (C1).

Fix: parse `X-Forwarded-For` (trusting only the proxy hop), or enforce limits at nginx.

### H3. `Hub.Broadcast` can stall the entire ingestion pipeline — FIXED
`websocket.go` `Broadcast` holds `h.mu.RLock()` for the whole client loop, and in the
"slow client" branch performs a **blocking** `client.send <- filtered` after draining one message.
`Broadcast` is called synchronously from `Batcher.flush`.

Impact: one slow/stuck WebSocket client whose 256-buffer stays full blocks the broadcast loop under
the read lock, which blocks `flush`, which blocks a batcher worker. Enough slow clients stall
ingestion and WAL truncation.

Fix: fully non-blocking send (drop batch on full buffer), move broadcast off the flush path
(dedicated goroutine + buffered channel), and don't hold the lock during sends.

### H4. WAL data loss when a flush fails then a later flush succeeds — FIXED
`batcher.go` `flush`: on `InsertBatch` error it unlocks and returns, correctly leaving the batch in
the WAL for recovery. But `wal.Append`/`wal.Truncate` operate on the whole file, and the **next**
successful flush (any worker) calls `w.Truncate(w.path, 0)`, deleting the still-un-inserted failed
batch.

Impact: silent loss of logs that failed to insert — the exact case the WAL exists to protect.

Fix: per-batch WAL records with offsets/acks, or only truncate entries confirmed inserted.

### H5. Panics in WebSocket goroutines crash the whole server — FIXED
`Recoverer` middleware only wraps the HTTP handler chain. `Client.writePump`/`readPump` and
`Hub.Run` run in bare goroutines. A `send on closed channel`, a nil deref, or a websocket library
panic there takes down the process (and with it all ingestion).

Fix: `recover()` in every long-lived goroutine; guard `client.send` sends against the closed state.

### H6. `CORS` allows any origin and `CheckOrigin` always returns true — FIXED
`middleware.go` sets `Access-Control-Allow-Origin: *` for all routes;
`websocket.go` `upgrader.CheckOrigin` returns `true` unconditionally (comment admits it).

Impact: any website can drive the API from a victim's browser (the API key is public anyway, C4)
and open live-tail sockets. Cross-site WebSocket hijacking.

Fix: allowlist origins for both CORS and the WS upgrader.

---

## Medium

> **Status: M1–M10 fixed** (2026-09-07). Notes at the end of this section.

### M1. No partition management — partitioning provides no benefit — FIXED
`init.sql` RANGE-partitions `logs` on `timestamp` but only creates `logs_default`. Nothing creates
weekly partitions. All rows land in one partition; the BRIN index degrades as timestamps interleave;
partition pruning never happens; retention drops become full deletes.

### M2. `Search`/`ListAlerts` do unbounded full scans — FIXED
`queries.go` `Search` with no `from`/`to` scans every partition, and `COUNT(*) OVER()` materialises
the full matching count on every request. `ORDER BY timestamp DESC, id DESC` across all data.
`GetLog` queries `WHERE id = $1` with no timestamp, forcing a scan of all partitions.
At volume these become slow and are reachable by any API-key holder (DoS-ish).

### M3. LIKE-injection / wildcard abuse in search — FIXED
`fuzzyQ := strings.ReplaceAll(q, " ", "%")` and `"%"+fuzzyQ+"%"` — user-supplied `%` and `_` are
not escaped. Not SQL injection (parameterised), but users can craft patterns that defeat the
trigram index and force sequential scans, or broaden matches unexpectedly.
Fix: `ESCAPE` clause and escape `% _ \` in input.

### M4. `healthHandler` writes the header twice — FIXED
`router.go`: `w.WriteHeader(http.StatusServiceUnavailable)` then `respondJSON(...)` calls
`w.WriteHeader` again → "superfluous WriteHeader call" logged, and status/body handling is fragile.
Fix: call `respondJSON` once.

### M5. Alert rule validation is thin — FIXED
`createRuleHandler`/`updateRuleHandler` validate only that `pattern` compiles and (create only) that
name/pattern are non-empty. No bounds on `cooldown_minutes` (0 or negative ⇒ alert fires on every
matching log ⇒ alert-table thrash + log spam), no limit on number of rules, no cap on pattern cost.
`updateRuleHandler` doesn't check `name` non-empty.

### M6. Unbounded regex evaluation cost in the alert engine — FIXED
`engine.Check` runs every compiled rule against every message in every batch while holding
`cacheMu.RLock()`. Go's RE2 is linear (no catastrophic backtracking), but many rules × high ingest
rate is O(rules × batch × msg-len) CPU on the hot path, serialised against rule reloads.

### M7. WAL corruption wedges startup recovery — FIXED
`wal.go` `Replay` returns an error on any malformed line (e.g. a torn write from a crash mid-append).
`main.go` only logs a warning and continues, but the WAL is never truncated, so it grows forever and
every subsequent restart re-hits the corrupt entry. `Append` also isn't crash-atomic (no temp-file
rename, partial line possible).

### M8. `InsertBatch` is not transactional — FIXED
`postgres.go` uses `pgx.Batch`; a failure at entry *k* leaves entries `0..k-1` committed and returns
an error. The caller (`flush`) then treats the whole batch as failed and leaves it all in the WAL →
on replay those first entries are inserted again (duplicates; there is no idempotency key at the DB
level).

### M9. JWT token travels in the URL query string — FIXED
`LiveTail.jsx` builds `…/ws/tail?token=${jwtToken}`. nginx's default access log records the full
request line, so tokens land in proxy logs. 60s expiry limits the window but it's still credential
material in logs. Prefer `Sec-WebSocket-Protocol` or a short-lived cookie.

### M10. `nginx.conf` proxies `/ws` but the backend route is `/ws/tail`; `/health`, `/metrics`, `/ingest` are not proxied — FIXED
Works today only because the frontend never calls those through nginx. Fragile and undocumented;
`/metrics` being unreachable through the proxy also means the compose healthcheck path assumptions
matter.

---

## Fix notes (M1–M10)

Verified: `go test ./...` (+`-race` on api/collector) and a full `docker compose up` —
partitions auto-created, rule-validation 400s, `%`/`_` escaped in search, health single 200,
transactional rollback on a bad batch, WS auth via `Sec-WebSocket-Protocol` (no token in URL),
`/health` through nginx, `/metrics` kept off nginx.

- **M1**: `storage.EnsurePartitions` provisions weekly partitions for a rolling window
  (1 back … 2 ahead), called at startup + every 6h via `RunPartitionMaintenance` (goroutine in
  `main.go`). Best-effort: a `CREATE` that clashes with existing `logs_default` rows is logged,
  not fatal — migrating an existing non-empty DB out of the default partition is manual.
- **M2**: `searchHandler` applies a default 30-day lower bound when `from` is absent, so an
  unfiltered query can't scan every partition. `GET /api/logs/{id}?ts=<RFC3339>` passes a ±1-day
  bound on the partition key to `Store.GetLog` for partition pruning (falls back to full scan
  when omitted). `COUNT(*) OVER()` kept but now bounded by the window.
- **M3**: `likePattern()` backslash-escapes `\ % _` then maps spaces to `%`; both `Search` and
  `ListAlerts` use `ILIKE $n ESCAPE ''`. Unit test `internal/storage/queries_test.go`.
- **M4**: `healthHandler` calls `respondJSON` once (no double `WriteHeader`).
- **M5**: `validateRule()` (shared by create + update): trims and requires name, caps name 200 /
  pattern 500, compiles the regex, requires `0 ≤ cooldown_minutes ≤ 10080`. `CreateRule` also
  rejects once `CountRules() ≥ 500` (`409`). Test `internal/api/rules_test.go`.
- **M6**: `engine.Check` snapshots the compiled-rule pointers under a brief `RLock` then matches
  without holding the lock — regex work no longer blocks (or is blocked by) `LoadRules`.
- **M7**: segmented WAL (from H4) + `Replay` now removes stray `*.json.tmp`, quarantines corrupt
  files as `*.corrupt`, and returns per-segment batches; `main.go` replays each segment in its
  own `InsertBatch` and `QuarantineSegments` (`*.failed`) any that permanently fail, so one
  poison batch can't wedge startup or grow the WAL unbounded.
- **M8**: `Store.InsertBatch` runs inside a `pgx.Tx` (`Begin`/`SendBatch`/`Commit`, `defer
  Rollback`). A mid-batch failure commits nothing, so a retained WAL segment replays without
  creating duplicates. Residual: a crash in the ~1-statement window between `Commit` and the
  segment delete can still re-insert one batch on restart (no DB-level idempotency key).
- **M9**: the WS JWT now rides in `Sec-WebSocket-Protocol` (`auth.<jwt>`, alongside a plain
  `logstream` subprotocol the server echoes); `?token=` still accepted for non-browser tools.
  `LiveTail.jsx` uses `new WebSocket(url, ['logstream', 'auth.'+jwt])`.
- **M10**: nginx `default.conf.template` proxies `/ws/` (was `/ws`) and adds `= /health`;
  `/ingest` and `/metrics` are documented as deliberately not exposed via the frontend.
  `proxy_read_timeout 3600s` on the WS location.

### Still open
Lows L1–L12. Residual on M8 as noted above.

---

## Low / hygiene

> **Status: L1–L12 fixed** (2026-09-07).

- **L1.** `docker-compose.yml` version `'3.8'` is obsolete; `vector-agent` mounts `./logs` which
  isn't in the repo and has no `.gitkeep`.
- **L2.** Dead schema: `search_vector` tsvector generated column + `idx_logs_search` GIN index are
  still created but unused after the move to trigram/ILIKE — pure write-amplification on every insert.
- **L3.** `.gitignore` has `*.log` **and** `wal.log`, plus a stray `.env.local.logstream-collector`
  entry (looks like a missing newline between `.env.local` and `.logstream-collector`). The
  committed `.logstream-collector` is a build artifact and should be ignored, not tracked.
- **L4.** `frontend/Dockerfile` bakes `http://localhost:8090` into the production bundle; the app
  only works when opened on the build host.
- **L5.** Logging is `fmt.Printf` to stdout throughout (no levels, no structure, no correlation IDs);
  ironic for a log-aggregation product and hard to operate.
- **L6.** `main.go` reads `port` only from `--port` (default 8090); there is no `PORT` env var, but
  compose passes `--port=8090` explicitly so it's consistent — still, `dbURL`/`apiKey`/`walPath`
  have env fallbacks and `port` doesn't.
- **L7.** No `Content-Type` check on `/ingest`; a client can POST anything and the JSON parser error
  is returned verbatim (`fmt.Sprintf("Failed to parse: %v", err)`) — minor information leak.
- **L8.** `stdin.go` reads `scanner.Text()` and sends on an unbuffered `lines` channel from a
  goroutine that isn't tied to `ctx`; on `ctx.Done()` that goroutine leaks until stdin EOF.
- **L9.** `idempCache` is a 10k-entry in-memory LRU with no TTL; under sustained load keys evict
  quickly and legitimate retries of an old request are re-processed. Not shared across replicas.
- **L10.** No tests anywhere in the repo.
- **L11.** `CORS` allowed headers list is `X-API-Key, Content-Type` but not `X-Idempotency-Key`,
  so browser clients can't send the idempotency header on cross-origin ingest.
- **L12.** `models.LogEntry.Metadata` is accepted from clients unbounded — a caller can attach
  arbitrarily large JSONB blobs to each of 1000 logs per 10 MB request.

---

## Suggested priority order

1. C1 (auth on /ingest) and C4/H1 (stop shipping the API key / JWT secret to browsers).
2. C2 (add the unique index so alerts actually persist).
3. C3 (refuse to boot with default secrets).
4. H3/H4/H5 (ingestion-pipeline stalls and WAL data loss).
5. H2/H6 (rate limiting behind proxy, origin allowlists).
6. Medium items, then hygiene.

---

## Fix notes (C1–C4)

- **C1**: `/ingest` moved into an authenticated + rate-limited `r.Group` in
  `internal/api/router.go`. New `INGEST_KEY` env (falls back to `API_KEY`) checked via
  `APIKeyAuth`; `RateLimit` now also applies. Vector agent, `dogfood.sh`, `loadtest.go`,
  and `ingest_ml_data.py` updated to send the key from the environment.
  Note: the load-test firehose will now see 429s once it exceeds 100 req/s per IP — expected.
- **C2**: `init.sql` adds `CREATE UNIQUE INDEX idx_alerts_rule_id ON alerts (rule_id)` so the
  `ON CONFLICT (rule_id)` upsert works. `docker-compose.yml` now mounts `init.sql` into the
  postgres init dir. Existing databases need the index added manually:
  `CREATE UNIQUE INDEX idx_alerts_rule_id ON alerts (rule_id);` (dedupe existing rows first).
- **C3**: `cmd/logstream/main.go` refuses to boot when `API_KEY`/`INGEST_KEY` resolve to
  `dev-key` or `DATABASE_URL` contains `:password@`, unless `ALLOW_INSECURE_DEFAULTS=true`.
  `.env.example` added; compose uses `${VAR:?}` interpolation.
- **C4**: The browser bundle no longer contains an API key. `frontend/src/config/api.js` sends
  no key in production; nginx (`frontend/default.conf.template`, rendered via envsubst with
  `LOGSTREAM_API_KEY`) injects `X-API-Key` on proxied `/api/` and `/ws/` requests and is the
  only same-origin path to the API. `VITE_API_KEY` remains an opt-in for `vite dev` only.
  Residual: anyone who can reach the dashboard origin still reaches the API unauthenticated
  from the browser's perspective — a real session/BFF auth layer is still the proper fix
  (tracked separately).

---

## Fix notes (H1–H6)

New env vars: `WS_JWT_SECRET`, `TRUSTED_PROXIES`, `ALLOWED_ORIGINS` (see `.env.example`
and `internal/api/security.go`). `internal/api/security_test.go` and
`internal/collector/wal_test.go` cover the tricky paths; verified end-to-end against a
local Postgres (ingest auth, CORS allow/deny, WS origin 403, forged/`alg=none`/expired
token 401, alert upsert `hit_count=2`).

- **H1**: WS tokens now signed/verified with a dedicated secret — `WS_JWT_SECRET`, or
  `sha256("logstream:ws-jwt:v1:" + API_KEY)` when unset (distinct from the raw key).
  `jwt.Parse` pins `WithValidMethods(["HS256"])` + `WithExpirationRequired()` and checks
  `token.Valid`. `wsTokenHandler` emits `RegisteredClaims`. A token signed with the API key,
  an `alg:none` token, and an expired token are all rejected (tested).
- **H2**: `RateLimit(trustedProxies)` resolves the client via `clientIP()` — walks
  `X-Forwarded-For` right-to-left, skipping trusted-proxy hops, only when the direct peer is
  itself trusted. Default trusted set = loopback + RFC1918 (covers the Docker bridge / nginx).
  nginx already forwards `X-Forwarded-For` / `X-Real-IP`.
- **H3**: The `Hub` clients map is now owned solely by the `Run` goroutine (register,
  unregister, fan-out). Producers call `Broadcast`, a non-blocking send into a buffered
  channel; per-client sends are non-blocking too. A slow/stuck client drops batches
  (`logstream_websocket_dropped_batches_total`) and can no longer block `flush` or ingestion.
  The batcher hands a fresh slice to the hub each flush (no shared-array race).
- **H4**: WAL is now one segment file per batch (`<WAL_PATH basename>.wal.d/seg-*.json`,
  fsync + atomic rename). A batch is deleted only after its own insert succeeds; a failed
  or crashed batch stays and is replayed at startup — no other flush can destroy it. Corrupt
  segments are quarantined as `*.corrupt`. `flushMu` removed (no longer needed → real worker
  parallelism). `WAL.Append`/`Truncate` replaced by `AppendSegment`/`RemoveSegment(s)`;
  `Replay` returns the segment paths to delete post-insert.
- **H5**: `Hub.Run` wraps its loop in a recover-and-restart; `writePump`/`readPump` recover
  and exit cleanly (connection still closed by their existing defers). The send-on-closed-
  channel path is gone because only `Run` closes `client.send` and only `Run` sends to it.
- **H6**: `CORS(allowedOrigins)` echoes the Origin only when allowlisted (`ALLOWED_ORIGINS`,
  `*` allowed for dev) or same-host; unknown origins get no CORS headers. The WS upgrader's
  `CheckOrigin` applies the same rule and `ServeWS` returns 403 before upgrading. compose
  sets `ALLOWED_ORIGINS=http://localhost:5173`.

### Still open from the original audit
Mediums M1–M12 and Lows L1–L12 are unchanged, except: **M8** (non-transactional `InsertBatch`
→ replay duplicates) is now the main residual WAL risk, and **M10**'s `/ws` path bug was fixed
in passing (nginx now proxies `/ws/`). **H2 note**: the load-test/ingest firehose is still
capped at 100 req/s per resolved client IP.

---

## Fix notes (L1–L12)

- **L1**: `docker-compose.yml` already dropped the obsolete `version:` key; added `logs/.gitkeep`
  and `.gitignore` rules so the vector-agent mount dir exists but its contents aren't tracked.
- **L2**: removed the unused `search_vector` tsvector generated column + `idx_logs_search` from
  `init.sql` (search is trigram/ILIKE on `message`). Migration note in the file for existing DBs.
- **L3**: `.gitignore` fixed — split the mangled `.env.local.logstream-collector` line into
  `.env.local` / `.env.*.local`, added `.logstream-collector`; the committed build artifact is
  untracked (`git rm --cached`).
- **L4**: already fixed in the C4 work — `frontend/Dockerfile` builds same-origin, bakes no URLs.
- **L5**: all server-side logging moved to `log/slog` via `internal/logging` (`LOG_FORMAT=json|text`,
  `LOG_LEVEL`). `RequestID` middleware assigns a correlation id (response `X-Request-ID`, context,
  and auto-added to every downstream log line). `Logger` emits one structured record per request.
  The `collect` CLI keeps plain stdout output on purpose.
- **L6**: `PORT` env is honoured when `--port` isn't passed.
- **L7**: `/ingest` rejects a non-`application/json` `Content-Type` with 415, and returns a generic
  `{"error":"invalid log payload"}` instead of echoing the raw parser error (logged server-side).
- **L8**: the `collect` stdin reader goroutine now selects on `ctx.Done()` when handing off a line,
  so it stops promptly instead of blocking on a send after shutdown.
- **L9**: idempotency cache is now `expirable.LRU` with a 10-minute TTL (was an unbounded-lifetime
  10k LRU); check-and-set is guarded by a mutex.
- **L10**: added tests — `internal/parser/parser_test.go`, `internal/collector/http_test.go`,
  plus the api/collector/storage tests from earlier phases. `go test ./...` is green.
- **L11**: already fixed in H6 — `X-Idempotency-Key` is in the CORS allowed-headers list.
- **L12**: `/ingest` rejects any entry whose JSON metadata exceeds 16 KiB (413), on top of the
  existing 10 MiB body / 1000-entry caps.

**Smoke-test finding (fixed):** `POST /api/rules` with `is_active` omitted created an *inactive* rule (Go bool zero-value shadowing the DB `DEFAULT TRUE`). Now defaults to active unless `is_active:false` is sent explicitly.

Nothing from the original audit remains open except the noted M8 residual (crash window between
tx commit and WAL-segment delete → at most one batch re-inserted on restart).
