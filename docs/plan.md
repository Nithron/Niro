# Nithro: Build Plan

*v1, **approved by the PM on 2026-11-22**. Changes after approval are recorded in [decisions](decisions.md).*
*Inputs: [decisions](decisions.md), [discovery research](research/2026-11-discovery.md), [Go library check](research/2026-11-go-library-check.md).*

## 0. Summary

- **Product:** a security-first, source-available Discord bot. Moderation, protection against compromised accounts, scams and raids, logging, roles, welcome and tickets, all managed from a web dashboard. Hosted by Nithron and self-hostable.
- **Stack:** Go + disgo (bot and API) in **one binary**, a React + Vite + TypeScript dashboard embedded in that binary, PostgreSQL, and Valkey.
- **Releases:**
  - **v0.1** (MVP, six milestones M0–M5)
  - **v0.2** Premium and payments
  - **v0.3** AI via nAI
  - **v0.4** Community extras and NithronOS bridge
  - **v0.5** Music
- **Repo goes public** with the first code release, `v0.1.0-alpha.1`, at the end of **M3**. By then moderation, Guardian and logging work end to end, which is the core value.
- **Way of working:** each milestone ends with a demo in the test server, and you approve it before the next one starts.

## 1. Principles

1. **Secure and private by default.** Request least-privilege permissions (never Administrator). Keep as little data as possible: message text is not stored in Postgres; the edit/delete log cache lives in Valkey with a TTL. Quarantine before banning, and a human confirms irreversible actions.
2. **Platform-agnostic core.** Domain packages never import disgo. Discord is one adapter, which keeps a Fluxer adapter or a library swap cheap.
3. **One codebase for hosted and self-hosted.** `nithro all` runs everything in one process for self-hosters. The hosted service runs `nithro bot` and `nithro api` separately so they can scale.
4. **Configuration is data.** Every setting is editable in the dashboard and applies live (pub/sub) without a restart. Every change is audited.
5. **The free core stays free.** Only features that cost us money to run (AI, long retention, higher limits) are premium.
6. **Tests first for domain logic,** especially the detection heuristics. Integration tests run against real Postgres and Valkey.

## 2. Architecture

```mermaid
flowchart LR
  D[(Discord\nGateway + REST)] <--> B[nithro bot\ndisgo, sharded]
  B <--> V[(Valkey\nconfig cache · pub/sub · counters\nmessage cache · job streams)]
  B <--> P[(PostgreSQL)]
  A[nithro api\nREST · OAuth2 · embedded dashboard] <--> P
  A <--> V
  U[Dashboard SPA\nin the browser] <--> A
  A -- user OAuth2 --> D
  B -. v0.3 .-> N[nAI]
  B -. v0.5 .-> L[Lavalink]
```

- **Only the bot process talks to Discord with the bot token.** When the dashboard needs a Discord action (post a role panel, create an AutoMod rule), the API puts a job on a Valkey stream and the bot executes it. That keeps a single rate-limit budget. The API only uses the *user's* OAuth token, to list their servers.
- **Config flow:** dashboard → API validates against the module's schema → Postgres (versioned, audited) → pub/sub `guild:{id}:config` → the bot hot-reloads that module for that guild.
- **Scale:** the bot runs a shard manager from day one (Discord requires shards at 2,500 servers). Multi-process clustering is designed for but not built until it's needed.

## 3. Repository layout

```
cmd/nithro/              single binary: nithro all | bot | api | migrate | version
internal/
  core/                  platform-agnostic types (guild, member, message, action) + module interfaces
  modules/               moderation/ guardian/ logging/ roles/ welcome/ tickets/
  platform/discord/      disgo adapter: events in, actions out, commands, components, emojis
  store/                 Postgres access (pgx + sqlc); migrations/ (goose)
  cache/                 Valkey: config cache, counters, message cache, job streams, pub/sub
  api/                   HTTP server, Discord OAuth2, sessions, authz
  config/ observability/ env config; slog logs, Prometheus metrics, health checks
api/openapi.yaml         API contract: generates the Go server interface and the TS client
web/                     dashboard (React + Vite + TS); built into web/dist, embedded with go:embed
deploy/                  Dockerfile, docker-compose (dev + self-host), reverse-proxy example
docs/                    plan, decisions, research, adr/, self-host guide
assets/brand/            logo and brand assets
```

## 4. Stack (versions current as of 2026-11-22)

| Area | Choice |
|---|---|
| Language | Go 1.27 |
| Discord | disgo v0.20 (gateway, REST, interactions, Components V2) |
| Database | PostgreSQL 18+, pgx v5.13, sqlc 1.31 (type-safe SQL), goose 3.30 (migrations) |
| Cache / queues | Valkey 8+, valkey-go |
| HTTP API | chi v5.5 + oapi-codegen v2.9 (spec-first, strict server) |
| Observability | `log/slog`, Prometheus client, health/readiness endpoints |
| Tests (Go) | `go test -race`, table-driven, testcontainers-go 0.45 for Postgres/Valkey, event-replay harness for Guardian |
| Dashboard | React 19, Vite, TypeScript, TanStack Router + Query, Tailwind CSS v4, Radix/shadcn-ui, openapi-typescript + openapi-fetch (same approach as the NithronOS web UI) |
| Tests (web) | Vitest, Playwright for critical flows |
| CI | GitHub Actions: golangci-lint, tests, `sqlc vet/diff`, OpenAPI lint, web lint/typecheck/test/build, Docker image build |

## 5. Data model (v0.1)

| Table | Holds |
|---|---|
| `guilds` | Servers the bot is in, join/leave dates, locale, plan tier |
| `guild_configs` | One row per (guild, module): JSONB config, version, who changed it and when |
| `config_audit` | Diff of every config change (dashboard or command) and who made it |
| `mod_cases` | Per-server numbered cases: warn, timeout, kick, ban, unban, quarantine, note; target, moderator, reason, duration, source (manual/AutoMod/Guardian) |
| `guardian_events` | Detection type, evidence *summary* (hashes, channel IDs, counts, not message text), action taken, who resolved it |
| `scam_domains` | Curated blocklist with its source; per-server allow/deny lists live in config |
| `role_panels` | Role panel definitions and their posted message IDs |
| `tickets`, `ticket_transcripts` | Ticket lifecycle; compressed HTML transcripts with an expiry date |
| Valkey only | Sessions, rate/raid counters, the **message cache** for edit/delete logs (TTL), job streams |

**Proposed retention defaults (for you to confirm):**
- Message cache: 7 days. Servers can turn content logging off.
- Guardian evidence: 90 days.
- Ticket transcripts: 90 days, configurable.
- Cases: kept while the bot is in the server.
- When the bot is removed, all of that server's data is deleted after a 30-day grace period.

## 5a. Languages (i18n)

v0.1 ships in **English only**, but it's built so another language can be added by adding translation files, with no code changes.

- **Bot:** every user-facing string is a message key in per-language catalogs (go-i18n, embedded in the binary); no hard-coded text in handlers.
  - **Ephemeral replies** use the invoking user's Discord locale.
  - **Public messages and logs** use the server's language: a dashboard setting that defaults to the server's Discord locale.
- **Slash commands:** names and descriptions use Discord's built-in `name_localizations` / `description_localizations` from the same catalogs.
- **Dashboard:** i18next + react-i18next with JSON namespaces; dates and numbers formatted through `Intl`.
- **CI** fails if a key is used but missing from the English catalog, and reports untranslated keys for other languages.
- **User content** (custom welcome texts, reasons) is stored as written and never translated automatically.

## 6. Discord application setup

- **Two applications:** **Nithro** (production, already created) and **Nithro Dev**, plus a private test server. You create Nithro Dev because it's tied to your account; I'll write the step-by-step.
- **Install context:** server install for v0.1. User install comes with the v0.3 AI features.
- **Gateway intents:**
  - Guilds, Guild Moderation, Guild Messages, AutoMod Configuration + Execution, Guild Voice States (voice logs).
  - **Server Members** (privileged): joins, for anti-raid and welcome.
  - **Message Content** (privileged): scam links, cross-channel spam, edit/delete logs.
  - **No Presence intent.**
- **Invite permissions:** View Channels, Send Messages, Embed Links, Attach Files, Read Message History, Manage Messages, Moderate Members, Kick, Ban, Manage Roles, Manage Channels, Manage Threads, Create Private Threads, View Audit Log, Manage Server (needed to manage AutoMod rules). **No Administrator.**
- **Verification:** an unverified bot can't join more than 100 servers. M5 prepares owner verification, the written justification for the privileged intents, ToS and Privacy URLs, and a support server.

## 7. v0.1 milestones

Sizes are relative (S/M/L), not calendar promises.

### M0: Foundations (L)
- Single binary with subcommands, env config, logging, metrics, health checks; dev `docker-compose` (Postgres, Valkey); migrations; CI pipeline.
- **Module framework:** registry, per-server enable/disable, config schema → validate → cache → live reload.
- **Discord adapter:**
  - Gateway plus shard manager.
  - Slash-command sync: server-scoped in dev, global in prod.
  - Interaction routing, Components V2 helpers.
  - Permission and role-hierarchy checks.
  - Upload of the brand status emojis as application emojis.
- **API + dashboard shell:** Discord OAuth2 login, sessions, server picker (servers where you have *Manage Server*, flagged if the bot is present), invite flow, branded layout, module toggles.
- **Done when:** Nithro Dev comes online in the test server and answers `/ping` and `/about`. You log in to the dashboard, toggle a module, and the bot reacts within about a second without a restart.

### M1: Moderation core (M)
- **Commands:** `/warn /timeout /untimeout /kick /ban /unban /purge` (filter by user, bots, links or count), `/case view|reason|delete`, `/history`, `/note`.
- **Cases:** numbered per server; optional DM to the user; mod-log posts using Components V2; scheduled temp-ban expiry.
- **Safety:** can't act on the owner, on yourself, or on someone with a higher role; bans need confirmation.
- **Native AutoMod management in the dashboard:** keyword/regex, mention-spam and spam rules; presets (slurs, invite links, scam phrases); exempt roles and channels.
- **Dashboard:** case browser with filters, case detail, moderation settings.
- **Done when:** the full case lifecycle works from both commands and dashboard, and AutoMod rules created in the dashboard appear in Discord's own AutoMod settings.

### M2: Guardian protection (L)
- **Compromised-account detection:**
  - The same or a near-identical message or link posted in ≥ N channels within T seconds.
  - A long-quiet account suddenly posting links.
  - Known scam templates (fake giveaways, "free Nitro", Steam gifts).
  - Thresholds are configurable per server, with sensible defaults.
- **Scam links:**
  - Curated domain blocklist with an updatable feed (licences of public lists to be checked).
  - Look-alike domain checks (punycode and homoglyphs of Discord, Steam, etc.).
  - Masked markdown links.
  - **Nithro never fetches user links** (privacy and SSRF safety).
- **Quarantine:**
  - Delete every copy of the message.
  - Time out the user (up to 28 days) or apply a quarantine role, and strip their roles (saved so they can be restored).
  - DM the user recovery steps.
  - Post a mod alert with **Restore / Ban / Keep quarantined** buttons.
- **Anti-raid:**
  - Join-velocity detection and an account-age gate.
  - Raid mode: pause invites through Discord's incident-actions API, raise the verification level, add slowmode.
  - `/lockdown` and `/unlock` per channel or for the whole server.
- **Verification gate:** a button check with an optional minimum account age (captcha comes later).
- **Permission audit:** `/guardian audit` plus a dashboard page that flags dangerous permissions on roles, bots and webhooks, using Discord's computed-permission endpoints.
- **Done when:** in the test server, a simulated compromised-account burst is quarantined within about 2 seconds with every copy deleted; a simulated raid triggers raid mode; and the false-positive review flow restores a user correctly.

### M3: Logging (M), then the first public release
- **Events logged:**
  - Message edits (before/after diff), deletes (from the cache), bulk deletes (attached as a file).
  - Joins and leaves (account age, invite used), nickname and role changes.
  - Bans, kicks and timeouts, with the executor from the audit log.
  - Channel, role and server changes (including the new position events), voice activity, AutoMod and Guardian actions.
- **Routing:** a destination channel per category; ignore lists for channels, roles and bots; per-server opt-out of message content.
- **Done when:** every event type produces a correct log entry in the test server.
- **Release:** tag `v0.1.0-alpha.1`, **make the repo public**, invite a few friendly servers as early testers.

### M4: Community toolkit (L)
- **Shared message builder:** a Components V2 builder in the dashboard with a live preview, reused by all three features below.
- **Role panels:** buttons or select menus; toggle, unique and verify modes; role limits; hierarchy checks.
- **Welcome / goodbye:** channel message and optional DM, variables (`{user}`, `{server}`, `{memberCount}`), auto-role on join (respecting the verification gate).
- **Tickets:**
  - Panels with categories; private-thread or channel mode.
  - Open from a button with an optional form.
  - Claim, close and reopen; staff roles; per-user limits.
  - HTML transcripts with retention.
- **Done when:** each feature works end to end when configured only from the dashboard.

### M5: Hardening and launch, `v0.1.0` (M)
- **Security:** review of the API and dashboard (authorization on every server endpoint, CSRF, sessions), dependency audit, fuzz tests on parsers.
- **Load/soak test** with synthetic gateway traffic; measure memory and CPU baselines; test shard resume.
- **Operations:** backups with a tested restore, retention jobs, cleanup after the bot leaves a server.
- **Self-hosting:** Docker image, compose file, `.env.example`, and a self-host guide (including the NCL commercial note), tested on a clean machine and on NithronOS.
- **Hosted:** production deployment with TLS, monitoring and alerts.
- **Legal and listing:** ToS, GDPR-compliant Privacy Policy, support server, Discord verification and privileged-intent applications, App Directory listing using the brand pack. User docs and changelog.
- **Done when:** `v0.1.0` is tagged, the hosted bot is verified and live, and the self-host guide works from scratch.

## 8. Roadmap after v0.1

| Release | Theme | Contents |
|---|---|---|
| **v0.2** | Premium and payments | Discord Premium Apps (SKUs) + Stripe checkout and customer portal, one entitlement model for both, premium gating, higher limits and retention, custom per-server bot nickname |
| **v0.3** | AI (nAI) | Q&A from the server's rules, pins, FAQ channels and uploaded docs, **with citations**; auto-answers in support forums; catch-up and thread summaries; translation; AI second opinion on borderline messages (flags only, never acts); bring your own model (Ollama, OpenAI-compatible, cloud); per-server quotas; user-installable commands |
| **v0.4** | Community+ and NithronOS | NithronOS bridge (SMART, pool health, backup and update alerts; nAI search); leveling (text + voice, role rewards); forms and applications; scheduled messages; anti-nuke auto-revert; opt-in shared scam-intelligence network |
| **v0.5** | Music | Lavalink + disgolink; Components V2 player and queue; SoundCloud, Bandcamp, internet radio, direct files, Jellyfin/Plex (NithronOS). **YouTube excluded by default** because of the ToS and takedown risk |
| Later | Reach | Fluxer adapter, more languages, public API and webhooks, server analytics |

## 9. How we'll work

- A branch and PR per feature into `main`; CI must pass. Commits are authored as you, with no AI trailers.
- Tests first for domain logic. Integration tests use real Postgres and Valkey. Playwright covers the dashboard's critical flows.
- Notable technical choices get a short ADR in `docs/adr/`.
- Each milestone ends with a demo in the test server and a changelog entry; you approve before the next milestone starts.
- graphify keeps the code knowledge graph current through its git hooks.

## 10. Risks

| Risk | Impact | Mitigation |
|---|---|---|
| Privileged intents (Message Content, Members) denied at verification | Guardian and logging weakened | Justify them clearly in writing; fall back to native AutoMod and audit-log data; degrade gracefully when an intent is missing |
| Guardian false positives | Angry users, lost trust | Quarantine instead of ban; one-click restore; conservative defaults; replay-tested heuristics; per-server tuning |
| disgo bus factor | Slower adoption of new Discord features | Adapter boundary; pin commits; contribute upstream |
| Scope creep before launch | Late v0.1 | Milestone gates with your approval; everything else stays on the roadmap |
| Hosting cost and outages | Reliability | Start with a single node plus backups; metrics and alerts from M0; clustering only when needed |
| GDPR (message logs, transcripts) | Legal exposure | Minimal retention, opt-outs, deletion on leave, privacy policy before launch |

## 11. Decisions before M0: resolved

1. Plan approved. 2. Nithro Dev application and test server created (configuration in [dev-setup](dev-setup.md)). 3. Retention defaults approved. 4. English first, built for more languages (§5a).
5. **Still open, needed by M5:** hosting provider and domain. See [domain options](research/2026-11-domains.md).
