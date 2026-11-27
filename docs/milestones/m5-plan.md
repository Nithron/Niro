# M5 implementation plan: Hardening and launch, `v0.1.0`

*2026-11-26. Built from plan §7 (M5), plan §5 and §6, decisions 4, 14 and the notes under the decisions table, and two audits of the M0–M4 code (an ECC security pre-audit of the API and dashboard, and an operations inventory).*

**Status (2026-11-26):** slices 1 to 7 are built and reviewed. Slice 8 (the `v0.1.0` tag) waits on the launch steps in [m5-demo.md](m5-demo.md) Part B.

**Done when** (plan §7): `v0.1.0` is tagged, the hosted bot is verified and live, and the self-host guide works from scratch.

## 1. Where things stand

**Already solid** (from the audits):
- Every guild-scoped API handler checks that the session user can manage that guild, and the checks fail closed. No missing check or cross-guild lookup was found in the API.
- Sessions:
  - The session ID is random, hashed in Valkey, and sent in an HttpOnly, SameSite=Lax cookie.
  - Discord's access token never reaches the browser.
  - OAuth uses a state cookie that is compared in constant time.
- CSRF: an Origin check on every non-GET request, with a `Sec-Fetch-Site` fallback.
- Security headers: a tight CSP, `frame-ancestors 'none'`, `nosniff`, and HSTS on https.
- Limits and secrets: a 64 KB request cap and server timeouts. No secrets in logs were seen.
- `niro all | bot | api | migrate` exist. The bot and API talk over Valkey RPC, so a split deployment works.
- `/healthz`, `/readyz` (Postgres, Valkey, Discord) and `/metrics` run on a separate ops port.
- Retention that already works: the message cache (7 days), sessions, the RPC stream, Guardian evidence (90 days) and ticket transcripts (hourly purge).
- Every guild table has `ON DELETE CASCADE` to `guilds`, so deleting one row removes all of a server's data.
- `deploy/Dockerfile` builds a distroless, non-root image.

**Gaps that block launch:**
- **No rate limiting** anywhere (a TODO in `internal/api/server.go`).
- **One reachable vulnerability:** GO-2026-6278 in gorilla/websocket v1.5.3, pulled in by disgo. The fixed version exists.
- **Leaving a server deletes nothing.** `forgetGuild` only sets `left_at`. Plan §5 promises deletion 30 days after the bot is removed.
- **Rows that are never cleaned up:**
  - `scheduled_actions` that are done.
  - Ended raids and lockdowns.
  - `config_audit`.
  - Guardian events still marked open or kept.
  - The Valkey key `guardian:since`.
- **No backups.**
- **No production compose file, reverse-proxy example or self-host guide.**
- **No alerts:**
  - No shard or gateway metrics.
  - No pool, REST 429 or job-success metrics.
  - `/readyz` passes when only one shard is up.
- **No fuzz tests**, no load or soak harness.
- **No ToS, Privacy Policy or listing material.**

## 2. Design

### 2.1 Security hardening
- **Rate limits** (Valkey, fixed one-minute windows). Defaults:
  - 120 requests a minute per user on `/api/v1`.
  - 30 a minute per user on requests that change something (anything but GET). These are the ones that write settings or reach the bot.
  - 10 a minute per IP on `/auth/login` and `/auth/callback`, and 60 a minute per IP on the public invite route.
  - If Valkey is down, requests are let through and a warning is logged.
  - The client IP is read from `X-Forwarded-For` only when the request comes from a configured trusted proxy (`NIRO_TRUSTED_PROXIES`).
  - Over the limit, the API returns 429 with `Retry-After`, and the dashboard shows a message.
- **Dependencies:**
  - Pin the fixed gorilla/websocket.
  - Add `govulncheck` and `pnpm audit --prod` to CI, so a reachable vulnerability fails the build.
- **An authorization test over the whole spec.** It walks every operation in `openapi.yaml` that has a guild path and asserts 401 without a session and 403 for a non-manager. A new endpoint that forgets the check fails CI.
- **The bot side gets the same check:** every RPC handler and store update filters on `guild_id AND id` (role and ticket panels, cases, AutoMod rules, Guardian events).
- **Sessions:**
  - The cookie gets the `__Host-` prefix.
  - A 24-hour idle timeout is added to the 7-day absolute limit.
  - Logout revokes the token at Discord.
  - A "Sign out everywhere" button is added.
  - The API refuses to start in production without an https public URL (it already does; a test is added).
- **Response hygiene:**
  - `Cache-Control: no-store` on `/api` and `/auth`.
  - `Permissions-Policy` and `Cross-Origin-Opener-Policy`.
  - `MaxHeaderBytes` lowered to 16 KB.
  - A bad request returns a generic message instead of the validator's text.
  - The public `/api/v1/invite` route stays an exact-match exception in the session check, covered by a test.
- **Fuzz tests** (Go native fuzzing; each target runs for 30 s in CI, and the seed corpus runs on every `go test`):
  - message documents (`Validate`, `Render`, `IsEmoji`);
  - `ParseDuration`;
  - Guardian link extraction and `NormalizeHost`, `normalizeText` and `normalizeDomains`;
  - the scam-feed parser;
  - the transcript renderer (it must never emit an unescaped tag);
  - the public URL setting.

  Custom IDs are parsed by disgo's router, not by Niro code, so they have no target of their own.
- **At the end:** an ECC security review of the whole M5 diff, with the same bar as M2 to M4.

### 2.2 Data lifecycle
- **When Niro leaves a server:**
  - The scheduler skips that server's pending timed actions (they run late if Niro comes back), and its Valkey keys are deleted at once. The message cache is keyed by channel and expires within its 7 days.
  - A daily job runs `DELETE FROM guilds WHERE left_at < now() - interval '30 days'`, and the cascade removes everything else. Migration 6 adds an index on `left_at`.
  - Rejoining within the 30 days restores everything, as today.
  - A `GuildLeave` caused by an outage (the server shows as unavailable) is not treated as a leave.
- **"Delete this server's data now":** a dashboard button for the server owner. Niro leaves the server and deletes its data immediately, with no grace period. This is the GDPR erasure path for a server.
- **New retention rules** (PM, question 4):
  - done `scheduled_actions` after 30 days;
  - ended raids and lockdowns after 90 days;
  - `config_audit` after 1 year;
  - Guardian events after 90 days, except ones a moderator marked **kept**, which stay while Niro is in the server, and ones still in effect: an **open** event holds a quarantine waiting for a moderator, an **active** one a raid. They go at the first purge after they are resolved;
  - `guardian:since` is deleted when Niro leaves (a TTL would restart Guardian's warm-up).
  - Lockdown rows are already removed when a channel is unlocked; a channel still locked keeps the overwrites it needs to be restored.
- **Every job** (maintenance, scheduler, scam feed, transcript purge, leave purge) records a last-success time and a failure counter, so a job that stops running raises an alert.

### 2.3 Observability
- **New metrics:**
  - per-shard status and heartbeat latency;
  - gateway events by type (`Ready` counts full reconnects, `Resumed` resumes);
  - Discord REST requests and time by route and status (429s included). Interaction and webhook tokens never reach a label;
  - Postgres pool use and wait time;
  - the rate limiter's Valkey errors (Valkey itself is covered by `/readyz`);
  - each job's last success, runs and failures;
  - build info.

  Bot-side RPC queue depth is left out: a bot that stops answering shows up as 502s in the API metrics.
- `/readyz` reports not-ready unless every shard is connected.
- **Shipped files** in `deploy/monitoring/`:
  - `alerts.yml` (Prometheus rules): Niro down, a shard down for more than 5 minutes, reconnect storms, 429 storms, Discord and API errors, the pool nearly full, a job overdue or failing, a backup overdue, and disk and memory.
  - `alerts_test.yml`, promtool unit tests that prove the main alerts fire (Niro stopped, a shard down, a stale job, a 429 storm).
  - `prometheus.yml`, `alertmanager.yml` (to a Discord webhook read from a file) and `grafana-dashboard.json`.
  - `make monitoring-check` runs the checks and tests in CI.

### 2.4 Load and soak test
- **`cmd/niro-soak`** (`make soak`):
  - It runs the real bot handlers against Postgres and Valkey in Docker.
  - It feeds synthetic gateway events through disgo's `EventManager.HandleGatewayEvent`.
  - Discord's REST API is replaced by a local fake server, through disgo's REST URL option.
  - Event mix: message creates, edits and deletes, joins (including raid bursts), and interactions.
- **Default target:**
  - 2,500 synthetic servers (the size where Discord requires sharding);
  - 100 messages a second with 50-join raid bursts;
  - a 2-hour soak.
- **Pass criteria:**
  - heap and goroutine counts stay flat after warm-up;
  - p99 handler latency stays under 50 ms;
  - nothing is dropped and there are no errors.
- **Results:** the measured memory and CPU baselines go into `docs/ops/baselines.md`.
- **Shard resume:** tested live on Niro Dev by forcing a gateway disconnect. The test checks that the session resumes (no full re-identify) and that no events are lost.

### 2.5 Self-hosting
- **`deploy/compose.yml`:**
  - `niro all`, Postgres 18 and Valkey 8 (with a password).
  - The ops port stays on the internal network.
  - A backup service.
  - Optional Caddy for automatic TLS (`deploy/Caddyfile.example`).
- **`.env.example`** gets a production section: the https public URL, trusted proxies, a warning to keep the ops port internal, and `NIRO_SCAM_FEEDS`.
- **Backups:**
  - Nightly `pg_dump --format=custom`, keeping 7 daily and 3 weekly copies. That is at most 28 days, so the 30-day leave purge also holds in backups.
  - Optional encryption with `age`.
  - `docs/ops/backup-restore.md`.
  - A CI job that dumps a seeded database, restores it into an empty one and checks the row counts.
  - Valkey holds only caches and sessions, so it is not backed up. A restart signs users out and empties the edit and delete cache.
- **`docs/self-host.md`:**
  - Discord application setup (intents, redirect URI, invite link), configuration, first start, upgrades and backups.
  - The NCL note: free for individuals and non-commercial communities, and a commercial licence for companies.
  - The trademark note: self-hosted instances use their own application and name.
- **Testing:** I test the guide from scratch on a clean VM. You test it on NithronOS (question 8).

### 2.6 Hosted deployment
- **`deploy/hosted/`:**
  - Compose for a single node, with a `niro migrate` init step, `niro bot` (one replica) and `niro api`.
  - Caddy with TLS for `niro.nithron.com`.
  - Prometheus, Alertmanager and Grafana (Grafana is private, reached over an SSH tunnel).
  - Off-site backups.
  - Alerts go to a private channel in the support server through a webhook, plus an external uptime check, so a dead server still alerts.
- **`docs/ops/runbook.md`:** deploying, rolling back, restoring, rotating the token and secrets, and what each alert means.
- **Hosting provider:** question 1.

### 2.7 Legal, listing and docs
- **Drafts of the Terms of Service and Privacy Policy**, written from the real data inventory:
  - what is stored, where, and for how long;
  - the transcript exception;
  - the 30-day leave purge;
  - backups;
  - the legal bases, the processors and your rights.
- **Published at `niro.nithron.com/terms` and `/privacy`** as public pages, served by the binary with no login.
- **They need your review**, and ideally a lawyer's. I'm not a lawyer.
- **Listing material:**
  - the written justification for the two privileged intents (Server Members, Message Content);
  - App Directory copy using the brand pack;
  - the support server's channel layout and Niro configuration (Niro moderates its own support server).
- **User docs:** `docs/user/`, one page per module. Changelog and release notes for `v0.1.0`.

### What only you can do
- **Hosting:** create the hosting account and the server, and the DNS records for `niro.nithron.com`.
- **Production Discord application (Niro):**
  - add the redirect URI;
  - owner identity verification;
  - the privileged-intent application;
  - the App Directory submission.
- **Support server:** create it. It's tied to your account. I'll write the setup.
- **Review:** the Terms of Service and Privacy Policy.
- **Testing:** run the self-host guide on NithronOS.

## 3. Slices

| # | Slice | Tests | Done check |
|---|---|---|---|
| 1 | **Security hardening:** rate limits, the vulnerability fix and audit jobs in CI, the spec-wide authorization test, bot-side scoping, sessions, headers, fuzz targets | Unit and integration tests for the limiter and sessions; the spec-walking authorization test; fuzz seeds; Vitest for 429 and sign-out everywhere | CI green, including govulncheck, the audit and fuzzing |
| 2 | **Data lifecycle:** leave handling and the 30-day purge, delete now, retention jobs, job metrics, migration 6 | Store integration tests (cascade purge, grace period, rejoin restores), domain tests, API tests | A server that Niro left is gone after the grace period (time-shifted test); delete now works in the test server |
| 3 | **Observability:** metrics, per-shard readiness, alert rules, Grafana dashboard | Metric tests; `promtool check rules` and `promtool test rules` in CI | The rule tests show the alerts fire when the bot is stopped, a shard drops or a job stalls |
| 4 | **Load and soak:** `niro-soak`, the fake REST server, baselines; shard resume on Niro Dev | The harness itself; a short version in CI | Baselines recorded; resume confirmed live |
| 5 | **Self-hosting and backups:** compose, Caddy, backup service, restore test, self-host guide | Restore test in CI; `docker compose config` in CI | Guide followed from scratch on a clean VM |
| 6 | **Hosted:** `deploy/hosted`, monitoring stack, runbook | — | The hosted bot runs on `niro.nithron.com` with TLS, alerts and backups |
| 7 | **Legal, listing, docs:** ToS, Privacy Policy, `/terms` and `/privacy` pages, intent justification, listing copy, support server setup, user docs | Vitest and Playwright for the public pages | Your review; verification and intents submitted |
| 8 | **Release:** final security review, changelog, `v0.1.0` tag | Full CI | Hosted bot verified and live; tag pushed; **demo with you** |

Slices 1 to 5 don't depend on the questions below, except question 4 (retention) for slice 2. Each slice gets an ECC review before it is merged into the branch.

**Timing:** Discord's verification and intent review is outside Nithron's control and can take days or weeks. Until it is approved, the hosted bot works in up to 100 servers, which is enough for a soft launch. The `v0.1.0` tag waits for the approval, as the done-when line says.

## 4. PM answers (decision 28)

1. **Hosting:** a HostStronger VPS, the same provider as the Nithron website. Off-site backups: optional, off at launch (decision 30).
2. **Monitoring:** Prometheus, Alertmanager and Grafana on the VPS.
3. **Data controller:** Nithron, Belgium, `privacy@nithron.com`. The supervisory authority is Belgium's Data Protection Authority.
4. **Retention:** as proposed.
5. **Self-host:** source only for `v0.1.0`.
6. **Delete now:** yes.
7. **Discord Team:** the production application already belongs to one.
8. **nOS** (formerly NithronOS): nOS ships Docker Engine and Compose v2 (its docs), so the clean-VM test covers it. Only HTTPS differs: nOS's Caddy holds ports 80 and 443 (self-host guide, "On nOS"). A native nOS catalog app would be separate work.

<details><summary>The questions as asked</summary>


1. **Hosting provider** (open since plan §11):
   - Recommended: Hetzner Cloud, one 4 vCPU / 8 GB server in Germany or Finland (about €15–20 a month), with Docker Compose, Caddy and off-site backups to a Hetzner Storage Box.
   - Data stays in the EU, which keeps GDPR simple.
   - A single node is enough for v0.1 (plan risks: "start with a single node plus backups").
2. **Monitoring:**
   - Recommended: Prometheus, Alertmanager and Grafana on the same server.
   - Alerts go to a private channel in the support server, plus a free external uptime check so a dead server still alerts.
   - The alternative is Grafana Cloud's free tier: less to run, but metrics leave Nithron's server.
3. **Legal details for the ToS and Privacy Policy:**
   - the legal entity's name and country (the data controller);
   - the contact addresses, for example `privacy@nithron.com` and `support@nithron.com`.
   
   Will you, or a lawyer, review the drafts?
4. **Retention gaps:**
   - done scheduled actions: 30 days;
   - ended raids and lockdowns: 90 days;
   - the config audit log: 1 year;
   - Guardian events: 90 days whatever their status, except ones a moderator marked kept, which stay while Niro is in the server.
   
   Do these defaults work?
5. **Self-host distribution for `v0.1.0`:**
   - Recommended: source only. The compose file builds the image from the repo. This matches the alpha (decision 21) and `COMMERCIAL.md`, where official build artifacts come later under the Binary EULA.
   - The alternative: publish an image on GHCR now.
6. **Delete now:** may a server's owner delete all of that server's data at once from the dashboard, which also removes Niro? Recommended. Otherwise erasure is by email only.
7. **Production Discord application:** should it be owned by a **Discord Team** instead of your personal account? Recommended, so ownership and verification don't depend on one account. Discord asks for this choice before verification.
8. **Load target and NithronOS:**
   - Is the load target OK: 2,500 servers, 100 messages a second, raid bursts and a 2-hour soak?
   - Will you run the self-host guide on NithronOS, or give me access to a NithronOS machine?

</details>
