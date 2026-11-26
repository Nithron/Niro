# Decisions log

| # | Date | Decision | Status |
|---|---|---|---|
| 1 | 2026-11-22 | **Direction:** security-first ("Guardian") + community toolkit at the core, AI assistant (nAI) as premium, NithronOS bridge as a free integration, with a platform-agnostic core | Decided |
| 2 | 2026-11-22 | **Language:** Go + disgo for the bot, Go for the API. See [Go library check](research/2026-11-go-library-check.md) | Decided |
| 2b | 2026-11-22 | **Dashboard:** TypeScript **React + Vite SPA** (same stack as the NithronOS web UI), not Next.js. It is embedded in the Go API binary, so self-hosting is one binary + Postgres + Valkey. Types are generated from the API's OpenAPI spec | Decided |
| 3 | 2026-11-22 | **License:** public repo under the Nithron Community License (NCL) v1.0 | Decided. Public since `v0.1.0-alpha.1` (2026-11-26, see #24) |
| 4 | 2026-11-22 | **Hosting:** both the official hosted bot and self-hosting | Decided |
| 5 | 2026-11-22 | **MVP (v0.1):** moderation + native AutoMod, compromised-account/scam/raid protection, logging with diffs, button roles, welcome, tickets, dashboard. **Music planned for a later release.** | Decided |
| 6 | 2026-11-22 | **Monetization:** Discord Premium Apps **and** Stripe | Decided |
| 7 | 2026-11-22 | **Logo:** concept A, the `<N>` brackets. Asset pack in `assets/brand/` | Decided |
| 8 | 2026-11-22 | **License text is shared:** `LICENSE` is byte-identical across NithronOS, NithronAI and Niro (bot added to nOS/nAI in NithronOS#2, NithronAI#1; renamed to Niro in all three, see #17) | Decided |
| 9 | 2026-11-22 | **Commits** are authored as the repo owner, with no AI attribution trailers (earlier history rewritten to match) | Decided |
| 10 | 2026-11-22 | **Repo goes public at the first code release.** The plan proposes `v0.1.0-alpha.1` at the end of M3 | Decided; timing in [plan](plan.md) |
| 11 | 2026-11-22 | **Build plan approved**, including milestone gates and the retention defaults in §5 | Decided |
| 12 | 2026-11-22 | **Languages:** English first; every user-facing string goes through message catalogs so other languages can be added later (plan §5a) | Decided |
| 13 | 2026-11-22 | **Dev app:** "Niro Dev" (created as "Nithro Dev") with the amber `<N>` DEV icon, plus a private test server | Done |
| 14 | 2026-11-22 | **Domain:** `niro.nithron.com` for the hosted dashboard and API (`nithro.nithron.com` was rejected as repetitive; earlier options in [domains](research/2026-11-domains.md)) | Decided; DNS set up in M5 |
| 15 | 2026-11-22 | **M0 accepted** (demo passed; logout fix merged in #2). M1 started | Done |
| 16 | 2026-11-22 | **Final milestone added: UI and brand polish** after all other milestones (dashboard UX, Discord message styling, logo/banner refresh) | Decided |
| 17 | 2026-11-22 | **Renamed Nithro → Niro** before M2: the bot will grow beyond security, so the name stays general. Repo `Nithron/Niro`, Go module `github.com/nithron/niro`, binary `niro`, env prefix `NIRO_`. The `<N>` mark is unchanged; the wordmark is redrawn as NIRO | Done |
| 17b | 2026-11-22 | **M1 accepted** (demo passed; merged in #3, rename in #5). M2 started | Done |
| 18 | 2026-11-22 | **Scam-domain feed:** default to nikolaischunk/discord-phishing-links (MIT); configurable with `NIRO_SCAM_FEEDS`. FishFish skipped (licence terms not found). Look-alike checks cover new domains. See [feeds](research/2026-11-scam-domain-feeds.md) | Decided |
| 19 | 2026-11-22 | **Guardian defaults:** enforce mode; quarantine = 7-day timeout; a member is quarantined only on strong signals (score ≥ 3), cross-posted plain text only alerts; raid = 10 joins in 30 s for 30 min. No Manage Webhooks permission (the audit says webhooks weren't checked) | Decided, tune after the demo |
| 20 | 2026-11-22 | **M2 accepted** (merged in #6). M3 is planned in [M3 plan](milestones/m3-plan.md) | Done |
| 21 | 2026-11-26 | **M3 choices:** every logged action goes to the log channel, Niro's own too (skipped only where the log channel *is* the moderation log, which already has the case card); the alpha release is source only; no CLA or DCO for contributions; a Playwright suite for the dashboard's critical flows before going public; the decisions log stands in for `docs/adr/` | Decided |
| 22 | 2026-11-26 | **Server log design (M3):** log entries are posted to Discord, not stored. Message text lives only in Valkey (7-day TTL, removed on delete). Moderation, member-update and server changes come from audit-log entries over the gateway, so each has its executor. Bulk deletes use a replaced disgo `MESSAGE_DELETE_BULK` handler (disgo splits them otherwise). Deletes of messages that weren't cached are not logged. M3 adds the non-privileged Guild Moderation, Guild Voice States, Guild Invites (missing from plan §6) and AutoMod Execution intents | Decided |
| 23 | 2026-11-26 | **M3 accepted** (demo passed; merged in #7). Release `v0.1.0-alpha.1` prepared: full-history secret scan (gitleaks, clean; now a CI job), `SECURITY.md`, `CONTRIBUTING.md`, issue and PR templates, `LICENSE` checked identical with NithronOS and NithronAI | Done |
| 24 | 2026-11-26 | **Repo public**, `v0.1.0-alpha.1` released (source only). Secret scanning with push protection, private vulnerability reporting, and branch protection on `main` (all CI checks required). M4 started: [M4 plan](milestones/m4-plan.md) | Done |
| 25 | 2026-11-26 | **M4 choices:** add Send Messages in Threads to the invite (thread-mode tickets); ticket transcripts are stored in Postgres (gzip HTML, 90 days by default, 1–365) and viewable in the dashboard, the one place Niro stores message text; the message builder takes image URLs only (no hosting); limits of 25 role panels, 10 ticket panels, 5 categories per panel, 1 open ticket per member by default; welcome DMs off by default | Decided |
| 26 | 2026-11-26 | **M4 design:** one message-document format (`internal/core/message`) for every designed message. Role panels and auto-roles refuse managed, `@everyone`, dangerous-permission and above-Niro roles, checked on save and on every click. Role menus toggle the picked roles. Picks use the interaction's roles with a per-member lock. Ticket transcripts are rendered with `html/template` and served with a `sandbox` CSP | Decided |
| 27 | 2026-11-26 | **M4 accepted** (demo passed; merged in #9). M5 started: [M5 plan](milestones/m5-plan.md) | Done |
| 28 | 2026-11-26 | **M5 choices:** hosting on a Hoststronger VPS (the provider of the Nithron website); Prometheus, Alertmanager and Grafana on that VPS; data controller is Nithron (Belgium), contact `privacy@nithron.com`; retention: done scheduled actions 30 days, ended raids and lockdowns 90 days, config audit 1 year, Guardian events 90 days except ones marked kept; self-host is source only for `v0.1.0`; server owners can delete their server's data at once from the dashboard; the production application already belongs to a Discord Team. NithronOS is now called **nOS** | Decided |

## Notes on these decisions

- **NCL is source-available, not OSI "open source".** Public marketing should say *source-available* or *open code*, not "open source".
- **NCL + self-hosting:** individuals and non-commercial communities can self-host for free. Under NCL §1/§3, a company or brand self-hosting Niro for its own server counts as *Commercial Use* and needs a commercial licence (`COMMERCIAL.md`). This is consistent with NithronOS/nAI, but it should be stated clearly on the self-host docs page.
- **Self-hosted instances run under the operator's own Discord application.** They can't use the Niro name or brand assets (Trademark Policy).
- **Before public launch, the hosted bot needs** a Terms of Service and Privacy Policy (Discord also requires these for App Directory listing and verification), plus GDPR handling, since we store message logs and moderation cases.
