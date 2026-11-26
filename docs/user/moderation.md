# Moderation

The Moderation module gives you commands for warnings, timeouts, kicks and bans. Every action becomes a numbered case, posted to your moderation log. It also manages Discord's built-in AutoMod from the dashboard.

## Set it up

Dashboard > your server > **Moderation**:

- **Moderation log channel**: every case is posted here. Niro needs to send messages in it.
- **Notify users by DM** (on by default): members get a private message when they are warned, timed out, kicked or banned.
- **Require a reason**: moderators must give a reason for every action.

## Commands

| Command | What it does | Default permission |
|---|---|---|
| `/warn user [reason] [silent]` | Warn a member | Timeout Members |
| `/timeout user duration [reason] [silent]` | Time out a member (1 minute to 28 days) | Timeout Members |
| `/untimeout user [reason]` | Remove a timeout | Timeout Members |
| `/kick user [reason] [silent]` | Kick a member | Kick Members |
| `/ban user [reason] [duration] [delete_messages] [silent]` | Ban a user, optionally for a set time | Ban Members |
| `/unban user [reason]` | Lift a ban | Ban Members |
| `/purge count [user] [bots] [links] [reason]` | Delete recent messages in this channel | Manage Messages |
| `/note user text` | Add a private moderator note | Timeout Members |
| `/history user` | Show a user's cases | Timeout Members |
| `/case view number` | Show one case | Timeout Members |
| `/case reason number reason` | Change a case's reason | Timeout Members |
| `/case delete number` | Delete a case from the history | Timeout Members |

Notes:

- **Durations** look like `10m`, `2h`, `3d` or `1w`. You can combine them, for example `1d12h`. Bans can last 1 minute to 365 days.
- **`/ban`, `/unban`, `/note` and `/history`** accept a user who isn't in the server: pick them or paste their user ID.
- **`/ban` asks you to confirm** before it bans. `delete_messages` can also remove the user's messages from the last hour, 24 hours or 7 days.
- **`silent:true`** skips the DM for that one action.
- **`/purge`** deletes 1 to 100 messages. With `user`, `bots` or `links` it only deletes matching messages. It never deletes pinned messages, and it skips messages older than 14 days (Discord doesn't allow bulk-deleting them).
- Reasons can be up to 512 characters.

## Safety checks

Niro refuses an action, and creates no case, when:

- you target yourself, the server owner or Niro;
- the member's highest role is not below yours;
- the member's highest role is not below Niro's (move Niro's role higher in **Server Settings > Roles**).

## Temporary bans

Give `/ban` a `duration`. When it runs out, Niro lifts the ban by itself and logs an unban case with the source **Automatic**. The user can then be invited again.

## Cases

Every action gets a case number, counted per server. A case records the action, the user, the moderator, the reason, the duration and where it came from:

| Source | Meaning |
|---|---|
| Command | A moderator used a slash command |
| AutoMod | Discord's AutoMod acted |
| Guardian | Guardian quarantined or restored a member |
| Dashboard | A moderator acted from the dashboard |
| Automatic | Niro did it on schedule, like ending a temporary ban |

Each case is posted as a card in the moderation log. `/case reason` updates that card in place. A deleted case is struck through as "deleted" in the log; it disappears from the history but stays in the audit trail.

`/history` lists a user's cases, with a link to see them all in the dashboard.

On the dashboard, the **Cases** tab lists every case, newest first. You can filter by **Action** or **User ID**, open a case, **Edit reason** or **Delete case**.

Cases are kept while Niro is in your server.

## AutoMod

The **AutoMod** tab manages Discord's own AutoMod. Rules run on Discord's side, instantly, even if Niro is offline. Rules you add here show up in **Server Settings > AutoMod** in Discord.

**Quick add** presets:

| Preset | What it does |
|---|---|
| Block invite links | Stops links to other Discord servers |
| Common scam phrases | Blocks "free Nitro", fake giveaways and similar bait |
| Mention spam | Blocks messages with 6+ mentions and times out the sender for 10 minutes |
| Suspected spam | Discord's own spam detection |
| Slurs and explicit content | Discord's maintained word lists |

Presets appear in Discord with a "Niro · " name, for example "Niro · Block invite links". You can adjust them later.

**New rule** lets you build your own:

- **Trigger**: Keywords (with `*` wildcards, optional regex patterns and allowed words), Suspected spam, Word lists (Profanity, Sexual content, Slurs) or Mention spam.
- **When triggered**: block the message (with an optional message shown to the member), send an alert to a channel, and time out the member (1 minute to 1 week).
- **Exempt roles** and **Exempt channels** are ignored by the rule.

You can turn a rule off with **Enabled**, edit it, or delete it. Rules made by Niro are marked **Created by Niro**.
