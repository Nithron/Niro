# Development setup

## 1. Configure the "Niro Dev" application

In the [Discord Developer Portal](https://discord.com/developers/applications), open **Niro Dev**:

| Page | Setting |
|---|---|
| **General Information** | App icon: `assets/brand/png/discord/dev-app-icon-1024.png`. Copy the **Application ID**. |
| **Installation** | Installation contexts: **Guild Install** only (untick User Install). Install link: *Discord Provided Link*. Guild install scopes: `bot`, `applications.commands`. Permissions: View Channels, Send Messages, Embed Links, Attach Files, Read Message History, Manage Messages, Moderate Members, Kick Members, Ban Members, Manage Roles, Manage Channels, Manage Threads, Create Private Threads, View Audit Log, Manage Server. |
| **OAuth2** | **Reset Secret** and copy the Client Secret. Redirects: add `http://localhost:8080/auth/callback`. |
| **Bot** | Banner: `assets/brand/png/discord/dev-profile-banner-1360x480.png`. **Public Bot: off** (only you can add it). Requires OAuth2 Code Grant: off. Privileged Gateway Intents: **Server Members on**, **Message Content on**, Presence off. **Reset Token** and copy it. |

Then add the bot to your test server with the install link (Installation page → copy link → open it → pick the test server).

To get the test server's ID: Discord → User Settings → Advanced → **Developer Mode** on, then right-click the server → **Copy Server ID**.

## 2. Put the secrets in `.env`

Copy `.env.example` to `.env` in the repo root and fill in the four Discord values. `.env` is git-ignored.
**Never paste the token or client secret into chat, issues or commits.** If one leaks, reset it in the portal.

```
NIRO_DISCORD_TOKEN=          # Bot → Reset Token
NIRO_DISCORD_CLIENT_ID=      # General Information → Application ID
NIRO_DISCORD_CLIENT_SECRET=  # OAuth2 → Client Secret
NIRO_DEV_GUILD_ID=           # test server ID (commands register there instantly)
```

## 3. Run it

Requirements: Go 1.27+, Node 22+ with pnpm 10, Docker (for Postgres and Valkey).

```
make dev-deps     # start Postgres + Valkey in Docker
make run          # build the dashboard, migrate, start bot + API on http://localhost:8080
```

For dashboard work with hot reload, set `NIRO_DASHBOARD_URL=http://localhost:5173` in `.env`, then run `make run-api` and `make web-dev` (Vite on http://localhost:5173, which proxies API calls to :8080).

Other useful targets: `make test` (Go tests, including Postgres/Valkey integration tests via Docker), `make lint`, `make generate` (after editing SQL queries or `api/openapi.yaml`), `make docker`, `make help`.

To check the milestone, follow [the M0 demo checklist](milestones/m0-demo.md).
