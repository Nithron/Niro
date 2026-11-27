# Changelog

All notable changes to Niro. Versions follow [Semantic Versioning](https://semver.org/).

## Unreleased

### M5: Hardening and launch

- **Rate limits:** the dashboard API limits requests per user and per IP, and the login flow per IP. Over the limit, the dashboard says to wait a moment. Behind a reverse proxy, set `NIRO_TRUSTED_PROXIES`.
- **Sessions:** dashboard sessions end after 24 hours without use (7 days at most). **Log out on all devices** is in the account menu. Logging out also revokes the Discord token. Over HTTPS the cookie uses the `__Host-` prefix.
- **Security:**
  - API answers are never cached, error messages don't echo internals, and there are new isolation headers.
  - Every server endpoint, and every database query on server data, is checked in CI to be scoped to the right server.
  - Fuzz tests run on every parser. They found and fixed two edge cases: overflowing durations and host names after IDNA mapping.
  - A vulnerable websocket library was updated, and CI now fails on known vulnerabilities in Go or dashboard dependencies.
- **Your data:**
  - A server's data is deleted 30 days after Niro is removed, including when that happened while Niro was offline. Re-adding Niro within that time keeps it.
  - Server owners can delete everything at once from the dashboard (**Delete data…**).
  - Old records expire: finished timed actions after 30 days, ended raids after 90 days, the settings history after 1 year.
- **Terms and privacy:** the Terms of Service and Privacy Policy are at `/terms` and `/privacy`, readable without logging in. The hosted Niro runs in Germany (HostStronger).
- **Monitoring:**
  - New metrics: shards, gateway events, Discord API requests and rate limits, the database pool, and every background job.
  - Readiness now needs every shard connected.
  - Ready-to-use Prometheus alert rules (with tests), an Alertmanager config and a Grafana dashboard are in `deploy/monitoring/`.
- **Self-hosting:**
  - `deploy/compose.yml` runs Niro with Postgres, Valkey, nightly encrypted-if-you-like backups and optional Caddy for HTTPS.
  - The [self-host guide](docs/self-host.md) and [backup and restore](docs/ops/backup-restore.md) explain it, including running on nOS.
  - Container logs rotate.
- **Load-tested:** `make soak` runs the real handlers against 2,500 synthetic servers. See [baselines](docs/ops/baselines.md).

### M4: Community toolkit

- **Message builder:** design messages in the dashboard with text, text-with-image sections, image galleries, spacers, link buttons and a colour bar, with a live preview that looks like Discord. Used by role panels, welcome and goodbye, and tickets. Images are linked, never uploaded or downloaded by Niro.
- **Role panels:** buttons or a menu that let members pick roles, in **Toggle**, **Pick one** or **Verify** mode (verify only adds, with Guardian's minimum account age), an optional limit per member and an optional required role. Up to 25 panels with 25 roles each. Niro never hands out managed roles, roles with dangerous permissions, or roles above its own. This is checked when saving and on every click. Editing a panel updates the posted message.
- **Welcome and goodbye:** a welcome message that may ping only the new member, an optional DM (off by default), a goodbye message, and up to 10 roles on join. Guardian comes first: members removed by the account-age gate aren't greeted. During raid mode, joins are summed up in one message. With the verification gate, roles are given on Verify.
- **Tickets:** panels with up to 5 categories. Each category opens a private thread or a private channel, has staff roles, an optional form of up to 5 questions, and a limit of open tickets per member. **Claim**, **Close** (with a reason), **Reopen** and **Delete** buttons, plus `/ticket add|remove|close|claim`.
- **Transcripts:** when a ticket closes, its messages are saved as an HTML transcript. Transcripts are kept 90 days by default (1–365), optionally posted to a channel, and viewable from the dashboard as a sandboxed page.
- **Invite:** adds **Send Messages in Threads**, for ticket threads. Existing servers re-invite once.

## v0.1.0-alpha.1 (2026-11-26)

First public release: moderation, Guardian and logging. Source only; container images and the self-host guide come with `v0.1.0`.

### M3: Logging

- **Message logs:** edits with the text before and after, deletions with the deleted text, and bulk deletions (`/purge`, Discord's own bulk delete) as one entry with a `.txt` file of the messages.
- **Member logs:** joins with the account's age (new accounts flagged) and the invite used, leaves with when they joined and their roles, nickname and role changes with who made them.
- **Moderation logs:** bans, unbans, kicks and timeouts, with who did it and the reason from Discord's audit log, including actions taken outside Niro.
- **Server logs:** channels and roles created, changed (including permissions and the new position changes) or deleted, and server setting changes.
- **Voice, AutoMod and Guardian:** voice joins, moves and leaves (off by default), AutoMod actions (one entry per trigger) and Guardian detections.
- **Routing:** a default log channel plus an optional channel per category, and every event type can be turned on or off. Ignore channels, roles and bots for message and voice logs. Niro never logs its own log channels.
- **No double posts:** Niro's own moderation actions are logged everywhere except in the moderation log channel, which already has the case card; Guardian detections aren't repeated in the Guardian alert channel.
- **Privacy:** message text is kept in Valkey for at most 7 days and dropped as soon as a message is deleted; it is never stored in the database. Servers can turn message text off entirely. Log entries never ping anyone.
- **Dashboard:** a Logging page with channels per category, event switches, ignore lists and the privacy switch.
- **Quality:** a Playwright suite covers the dashboard's critical flows in CI, and CI now lints the OpenAPI spec and vets the SQL queries.

### M2: Guardian protection

- **Hacked accounts:** the same message, link or file posted in several channels within a minute is caught; every copy is deleted and the member is quarantined (a timeout, or swapping their roles for a quarantine role). A member silent for a month who suddenly posts links adds suspicion to other signals.
- **Scam links:** a scam-domain blocklist (about 22,000 domains, refreshed every 6 hours), look-alikes of Discord, Steam and other brands (`dlscord.com`, Cyrillic letters, `discord-nitro.gift`), links whose text shows a different site, typical scam wording ("free nitro", "$50 steam gift", "I accidentally reported you"), and per-server allow and block lists. Niro never opens the links.
- **Review flow:** each quarantine posts an alert with **Restore**, **Ban** and **Keep quarantined** buttons (also on the dashboard). Restore gives back the member's roles or lifts the timeout. Quarantines and releases are moderation cases. Quarantined members get a DM with steps to secure their account.
- **Anti-raid:** a burst of joins turns on raid mode: invites are paused, the verification level goes to High, and optional slowmode is set. It ends on its own or with a button, and undoes exactly what it changed. `/guardian raid start|end|status` to control it by hand. Optional minimum account age on join (alert, kick or quarantine).
- **Lockdown:** `/lockdown channel|server` and `/unlock channel|server`, restoring each channel's exact previous permissions.
- **Verification gate:** `/guardian verify-panel` posts a **Verify** button that gives a role, optionally only to accounts older than a set age.
- **Permission audit:** `/guardian audit` and a dashboard page flag dangerous permissions on @everyone, roles and bots, webhooks, a low verification level, and moderators without 2FA.
- **Monitor mode** to try Guardian without it acting, trusted roles that are never checked, and moderators and the owner are always exempt.
- **Dashboard:** Guardian settings, Detections (with review buttons and a raid-mode switch) and Security audit pages.
- **Privacy:** message text is never stored. Detections keep a summary (channels, counts, hashes, domains) for 90 days.

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
