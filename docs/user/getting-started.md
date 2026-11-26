# Getting started

This page takes you from nothing to a working Niro in your server.

## Before you start

You need to be the server owner, or have the **Manage Server** permission (Administrators have it). The dashboard only shows servers where you have it.

## Add Niro to your server

1. Open https://niro.nithron.com and press **Log in with Discord**. Niro only reads your username, avatar and server list. It never sees your password or messages.
2. Under **Your servers**, find your server and press **Add Niro**.
3. Discord opens with your server already selected. Check the permissions and press **Authorize**.
4. Back on the dashboard, the server now shows **Manage**. If it doesn't, wait up to a minute and reload.
5. In Discord, run `/ping`. Niro answers with its response time. `/about` shows the version and a link to the dashboard.

## Permissions Niro asks for

Niro asks for each permission it needs, and nothing more. It never asks for **Administrator**. If you gave Niro's role Administrator yourself, the security audit tells you to remove it.

| Permission | Why |
|---|---|
| View Channels, Send Messages, Embed Links | Reply to commands, post logs, alerts and panels |
| Attach Files | Bulk-delete logs and ticket transcripts are sent as files |
| Read Message History | `/purge` and ticket transcripts |
| Manage Messages | Delete messages (`/purge`, scam messages) |
| Timeout Members | Timeouts and Guardian quarantine |
| Kick Members, Ban Members | `/kick`, `/ban`, `/unban`, temporary bans, the account-age gate |
| Manage Roles | Role panels, roles on join, the verified and quarantine roles, lockdowns |
| Manage Channels | Lockdowns, slowmode during raid mode, ticket channels |
| Manage Threads, Create Private Threads, Send Messages in Threads | Tickets as private threads |
| View Audit Log | Logs show who made a change |
| Manage Server | AutoMod rules, pausing invites and raising the verification level in raid mode, seeing which invite a member used |

Niro does not ask for Manage Webhooks, so the security audit can't list your webhooks.

## Put Niro's role high enough

Discord only lets a bot change members and roles that are **below its own highest role**. In **Server Settings > Roles**, drag Niro's role above:

- every role it should give out (role panels, roles on join, the verified role, the quarantine role);
- the highest role of any member it should moderate or quarantine.

Niro can never moderate the server owner. If its role is too low, it tells you, for example: "My highest role must be above (member)'s highest role."

## Turn modules on

1. On the dashboard, open your server. The first tab is **Modules**.
2. Switch modules on or off. Changes apply in Discord immediately, with no restart.
3. Open each module's tab to set it up. Settings are kept when a module is off.

Moderation and Guardian are on by default. Logging, Role panels, Welcome and Tickets start off. In Discord, `/modules` shows which modules are on.

Good first steps:

1. **Moderation** tab: choose a **Moderation log channel**. See [Moderation](moderation.md).
2. **Guardian** tab: choose an **Alert channel** only moderators can see. See [Guardian](guardian.md).
3. **Security audit** tab: fix what it finds.
4. Turn on **Logging** and pick a **Default log channel**. See [Logging](logging.md).

## Who can use which command

Each command has a default permission. For example `/warn` needs Timeout Members and `/ban` needs Ban Members; the commands are listed with their permissions on each page. To change who sees a command, use **Server Settings > Integrations > Niro**.

Anyone who can open the dashboard for a server (the owner, Administrators and members with Manage Server) can change all of its settings.

## Languages

Niro is in English for now. It is built so other languages can be added later without changes to how you use it:

- Replies only you can see follow your own Discord language.
- Public messages, logs and DMs follow your server's language in Discord.
- The dashboard follows your browser's language.

Text you write yourself (welcome messages, reasons, panel text) is posted exactly as you wrote it and never translated.
