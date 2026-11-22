# Changelog

All notable changes to Niro. Versions follow [Semantic Versioning](https://semver.org/).

## Unreleased

### M1: Moderation

- **Commands:** `/warn`, `/timeout`, `/untimeout`, `/kick`, `/ban` (permanent or temporary, optionally deleting recent messages, with a confirmation button), `/unban`, `/purge` (1–100 messages, optionally only from one user, only bots, or only links; pinned and 14+ day old messages are skipped), `/note`, `/history` and `/case view|reason|delete`.
- **Cases:** every action is a numbered case per server, with the moderator, reason, duration and source. Temporary bans are lifted automatically, and a manual unban cancels the pending one.
- **Safety:** nobody can act on themselves, the owner, the bot, or anyone at or above their own highest role. The bot explains when its role is too low instead of failing silently. Discord's audit log shows who acted and why.
- **Notifications:** members get a private message for warnings, timeouts, kicks and bans (before kicks and bans, so it still arrives). Servers can turn this off, and moderators can skip it per action with `silent`. Every case can be posted to a moderation log channel; editing or deleting a case updates its log message. Logs never ping anyone.
- **Dashboard:** Moderation settings (log channel, DMs, require a reason), a case browser with filters and paging, case detail with reason editing and deletion, and AutoMod management: five one-click presets (invite links, scam phrases, mention spam, spam, slurs/explicit word lists), a rule editor, enable/disable and delete. AutoMod rules are Discord's own, so they keep working even if Niro is offline.
- **Under the hood:** a request bridge so the dashboard can ask the bot to act (the bot token never leaves the bot), per-module settings with an audit trail, and integration tests against real Postgres and Valkey.

### M0: Foundations

- **Bot:** connects with automatic sharding; `/ping`, `/about` and `/modules` (Manage Server only), built with Components V2 and Niro's brand emojis (uploaded to the app automatically). Commands register instantly in a dev server or globally in production.
- **Modules:** Core, Moderation, Guardian, Logging, Role panels, Welcome and Tickets can be switched on and off per server. Their features arrive in M1–M4.
- **Live configuration:** changes made in the dashboard reach every bot process within about a second (Valkey pub/sub), with no restart. Every change is audited (who, what, from where).
- **Dashboard:** Discord login, a list of the servers you can manage (with an "Add Niro" invite link), and module switches that save instantly and roll back if saving fails. Dark and light themes follow your system.
- **Security:** least-privilege invite (no Administrator), server-side sessions with hashed IDs, OAuth state check, cross-site request blocking, strict security headers. The bot token never reaches the API process.
- **Languages:** English, with every user-facing string in translation catalogs (bot and dashboard) so more languages can be added without code changes.
- **Operations:** one `niro` binary (`all | bot | api | migrate | version`), `/healthz`, `/readyz` and Prometheus `/metrics`, graceful shutdown, a 51 MB distroless container image, and CI with lint, tests (including Postgres/Valkey integration tests), a generated-code check and an image build.
