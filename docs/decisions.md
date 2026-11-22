# Decisions log

| # | Date | Decision | Status |
|---|---|---|---|
| 1 | 2026-11-22 | **Direction:** security-first ("Guardian") + community toolkit at the core, AI assistant (nAI) as premium, NithronOS bridge as a free integration, with a platform-agnostic core | Decided |
| 2 | 2026-11-22 | **Language:** Go + disgo for the bot, Go for the API. See [Go library check](research/2026-11-go-library-check.md) | Decided |
| 2b | 2026-11-22 | **Dashboard:** TypeScript **React + Vite SPA** (same stack as the NithronOS web UI), not Next.js. It is embedded in the Go API binary, so self-hosting is one binary + Postgres + Valkey. Types are generated from the API's OpenAPI spec | Recommended; PM can override |
| 3 | 2026-11-22 | **License:** public repo under the Nithron Community License (NCL) v1.0 | Decided (repo still private until first public-ready commit) |
| 4 | 2026-11-22 | **Hosting:** both the official hosted bot and self-hosting | Decided |
| 5 | 2026-11-22 | **MVP (v0.1):** moderation + native AutoMod, compromised-account/scam/raid protection, logging with diffs, button roles, welcome, tickets, dashboard. **Music planned for a later release.** | Decided |
| 6 | 2026-11-22 | **Monetization:** Discord Premium Apps **and** Stripe | Decided |
| 7 | 2026-11-22 | **Logo:** concept A, the `<N>` brackets. Asset pack in `assets/brand/` | Decided |
| 8 | 2026-11-22 | **License text is shared:** `LICENSE` is byte-identical across NithronOS, NithronAI and Nithro (Nithro added to nOS/nAI in NithronOS#2, NithronAI#1) | Decided |
| 9 | 2026-11-22 | **Commits** are authored as the repo owner, with no AI attribution trailers | Decided |

## Notes on these decisions

- **NCL is source-available, not OSI "open source".** Public marketing should say *source-available* or *open code*, not "open source".
- **NCL + self-hosting:** individuals and non-commercial communities can self-host for free. Under NCL §1/§3, a company or brand self-hosting Nithro for its own server counts as *Commercial Use* and needs a commercial licence (`COMMERCIAL.md`). This is consistent with NithronOS/nAI, but it should be stated clearly on the self-host docs page.
- **Self-hosted instances run under the operator's own Discord application.** They can't use the Nithro name or brand assets (Trademark Policy).
- **Before public launch, the hosted bot needs** a Terms of Service and Privacy Policy (Discord also requires these for App Directory listing and verification), plus GDPR handling, since we store message logs and moderation cases.
