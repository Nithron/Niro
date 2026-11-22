# Go vs JS: how far behind are the Go Discord libraries?

*Checked 2026-11-22 against each project's git history and GitHub releases, so these are primary sources rather than blog claims.*

## Short answer

The lag I flagged in the discovery report is real for **discordgo** and **arikawa**. It is **not** real for **disgo**, which adds new Discord API features about as fast as discord.js and sometimes faster. If we pick Go, we pick **disgo**.

## Feature landing dates

Days are counted from the date Discord documented the feature (commit in `discord/discord-api-docs`) to the first library commit that implemented it.

| Feature | Discord docs | discord.js | **disgo** | discordgo | arikawa |
|---|---|---|---|---|---|
| Polls | 2024-04-18 | 2024-04-30 (+12 d) | **2024-04-22 (+4 d)** | 2024-06-21 (+64 d) | 2026-10-29 (+2.5 yr) |
| Components V2 | 2025-04-22 | 2025-04-23 builders, 2025-05-11 full (+19 d) | **2025-05-16 (+24 d)** | 2025-05-22 (+30 d) | 2026-02-24 (+10 mo) |
| DAVE E2EE voice (**mandatory since 2026-03-01**) | 2024-09-17 | 2025-07-13 | **2026-02-11, released v0.19.0 2026-02-12** (17 days before the deadline) | **none** | **none** |
| Audit log: channel/role position events | 2026-10-29 | not seen yet | **released in v0.20.0 2026-11-06 (+8 d)** | – | – |
| Checkbox component | late 2026 | – | **on master 2026-11-15** | – | – |
| Monetization (entitlements, SKUs, subscriptions) | – | 2024 | supported (fixes in v0.19/v0.20) | 2024-12 | – |

## Project health (2026-11-22)

| Library | Stars | Last push | Latest release | Open issues | Verdict |
|---|---|---|---|---|---|
| discord.js | 26.8k | today | 14.28.1 (2026-11-03) | 141 | Reference implementation, very active |
| **disgo** | 632 | 2026-11-16 | v0.20.0 (2026-11-06) | 13 | **Active; fast feature work; healthy issue count** |
| discordgo | 6.0k | 2026-02-14 | v0.29.0 (2025-05-24) | 234 | **Stagnating. No DAVE support, so voice hasn't worked with it since 2026-03-01. Avoid.** |
| arikawa | 609 | 2026-11-16 | v3.6.0 (2025-09-04) | 38 | Active but slow on features; avoid |

## The real trade-offs of Go + disgo

1. **Releases are infrequent, not features.** Features land on `master` quickly, but tagged releases can be months apart (v0.19.6 in June, then v0.20.0 in November). Go modules can pin a specific commit, so this doesn't block us.
2. **Small community and a small core team (bus factor).** There are far fewer tutorials and Stack Overflow answers than for discord.js. *Mitigations:* the planned platform-agnostic core (`domain` doesn't import disgo), a thin adapter layer, and willingness to contribute upstream (the project accepts outside PRs actively).
3. **The dashboard is still TypeScript** (Next.js) whichever bot language we choose. With Go, that's two languages, which is exactly how Flow is built (Go backend + Next.js).
4. **Brand-new endpoints may need a raw REST call** for a few days. disgo's REST client lets us call custom routes, so this is minor.

## Music (planned for later)

Music doesn't depend on the bot library's own voice code. The standard approach is **Lavalink** (a separate audio server):
- Lavalink added DAVE in **4.2.0 (2026-02-28)**.
- **disgolink** (disgo's Lavalink client) shipped DAVE-compatible **v3.1.0 the same day**.

So Go is fine for music. The bigger music risk is **content sources rather than code**. YouTube shut down Groovy and Rythm in 2021, and YouTube playback through Lavalink is an ongoing cat-and-mouse game. Safer sources are SoundCloud, Bandcamp, internet radio, direct files, and the **user's own Jellyfin/Plex library on NithronOS**, which fits the Nithron ecosystem well.

## Recommendation

**Go + disgo is a sound choice**, and it matches your preference and the Flow/NithronOS backends. In practice the "lag" only applies to discordgo. Revised stack if you confirm:

- `apps/bot`: Go + disgo (gateway, interactions, sharding)
- `apps/api`: Go (chi or echo) REST API, shared with the dashboard
- `apps/dashboard`: Next.js + Tailwind + shadcn/ui
- Data: PostgreSQL (+ pgvector), Valkey; Go access with sqlc + pgx; migrations with goose or Atlas
- Music (later): Lavalink + disgolink

### Sources
- Discord API docs history: https://github.com/discord/discord-api-docs/commits/main
- disgo releases: https://github.com/disgoorg/disgo/releases (v0.19.0 DAVE note, v0.20.0 changelog)
- discord.js releases: https://github.com/discordjs/discord.js/releases
- discordgo releases: https://github.com/bwmarrin/discordgo/releases
- arikawa releases: https://github.com/diamondburned/arikawa/releases
- Lavalink 4.2.0 (DAVE): https://github.com/lavalink-devs/Lavalink/releases
- disgolink: https://github.com/disgoorg/disgolink/releases
