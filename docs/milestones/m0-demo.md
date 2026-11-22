# M0 demo checklist

**Done when** (plan §7): Niro Dev comes online in the test server and answers `/ping` and `/about`. You log in to the dashboard, toggle a module, and the bot reflects it within about a second without a restart.

## Setup (once)

1. Configure the Niro Dev application and create `.env`: [dev-setup](../dev-setup.md) steps 1–2.
2. `make dev-deps`, then `make run`. Wait for `bot online` and `dashboard available` in the log.

## Demo

| # | Do | Expect |
|---|---|---|
| 1 | In the test server, type `/` | `/ping`, `/about`, `/modules` are listed under Niro Dev |
| 2 | `/ping` | Private reply "Pong! Gateway latency: N ms" with a green accent |
| 3 | `/about` | Private Niro card with the version and dashboard/website links |
| 4 | `/modules` | Modules list: Core always on, Moderation and Guardian enabled, the others disabled |
| 5 | Open http://localhost:8080 and log in with Discord | Your servers, the test server first with **Manage** |
| 6 | Manage → switch **Logging** on | Switch turns on, "Saved" appears; the bot log shows `config change received … module=logging` |
| 7 | Run `/modules` again | Logging is now **Enabled**: no restart, live within about a second |
| 8 | Optional: http://localhost:9090/readyz and `/metrics` | `ready: true` with postgres, valkey and discord checks; `niro_bot_interactions_total` counts your commands |

## Known limits (by design for M0)

- Module switches only turn modules on and off. Their features and settings arrive in M1–M4.
- If Niro is removed from a server while the bot is offline, the dashboard keeps showing it as present until the next time Discord reports it. Reconciliation is part of M5 (cleanup after leaving a server).
