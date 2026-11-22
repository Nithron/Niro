# Decisions log

| # | Date | Decision | Status |
|---|---|---|---|
| 1 | 2026-11-22 | **Direction:** security-first ("Guardian") + community toolkit at the core, AI assistant (nAI) as premium, NithronOS bridge as a free integration, with a platform-agnostic core | Decided |
| 2 | 2026-11-22 | **Language:** Go + disgo for the bot, Go for the API. See [Go library check](research/2026-11-go-library-check.md) | Decided |
| 2b | 2026-11-22 | **Dashboard:** TypeScript **React + Vite SPA** (same stack as the NithronOS web UI), not Next.js. It is embedded in the Go API binary, so self-hosting is one binary + Postgres + Valkey. Types are generated from the API's OpenAPI spec | Decided |
| 3 | 2026-11-22 | **License:** public repo under the Nithron Community License (NCL) v1.0 | Decided. The repo stays private until the first code release (see #10) |
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

## Notes on these decisions

- **NCL is source-available, not OSI "open source".** Public marketing should say *source-available* or *open code*, not "open source".
- **NCL + self-hosting:** individuals and non-commercial communities can self-host for free. Under NCL §1/§3, a company or brand self-hosting Niro for its own server counts as *Commercial Use* and needs a commercial licence (`COMMERCIAL.md`). This is consistent with NithronOS/nAI, but it should be stated clearly on the self-host docs page.
- **Self-hosted instances run under the operator's own Discord application.** They can't use the Niro name or brand assets (Trademark Policy).
- **Before public launch, the hosted bot needs** a Terms of Service and Privacy Policy (Discord also requires these for App Directory listing and verification), plus GDPR handling, since we store message logs and moderation cases.
