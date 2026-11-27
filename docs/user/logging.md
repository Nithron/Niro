# Logging

The Logging module posts what happens in your server to log channels you choose: edited and deleted messages, member changes, moderation, server changes, voice activity, AutoMod and Guardian actions. Log entries are posted to Discord; Niro doesn't keep its own copy.

## Set it up

1. Create one or more log channels that only moderators can see. Niro must be able to view them, send messages and attach files.
2. Dashboard > your server > **Modules**: turn **Logging** on.
3. **Logging** tab > **Log channels**: pick a **Default log channel**. Every category without its own channel goes there.
4. Optionally give a category its own channel, or set it to **None (off)**.
5. Under **What to log**, untick events you don't want. Save.

If no log channel is set, nothing is logged.

## Categories and events

| Category | Events | On by default |
|---|---|---|
| Messages | Edited messages (before and after), deleted messages, bulk deletes (as a file) | Yes |
| Members | Joins (account age and invite used), leaves, nickname changes, role changes | Yes |
| Moderation | Bans, unbans, kicks, timeouts | Yes |
| Server changes | Channels and roles created, changed (including permissions and order) or deleted; server settings changed | Yes |
| Voice | Voice joins, leaves and moves | No (it is noisy) |
| AutoMod | AutoMod actions | Yes |
| Guardian | Guardian detections | Yes |

Details:

- **Who did it**: moderation, member and server changes show who made them, from Discord's audit log. Discord doesn't say who deleted a single message, so deletes only name Niro when Niro did it.
- **Bulk deletes** (for example from `/purge`) are one entry with a `.txt` file listing the messages.
- **Joins** show the account's age, flag accounts less than a week old, and show the invite code and who created it. When Niro can't tell (two people joined at once, or a vanity URL), it says "unknown" instead of guessing.
- **Moderation entries** cover actions from Niro and from Discord's own menus. If the Moderation category goes to the same channel as your [moderation log](moderation.md), Niro's own actions are not posted twice: the case card is already there.
- **AutoMod** posts one entry per triggered rule, not one per action.
- Niro's own log channels are never logged.

## Message text

To show what an edited or deleted message said, Niro keeps message text in a short-lived cache (Valkey, not its database):

- it is kept for at most **7 days**;
- it is removed as soon as the message is deleted;
- it is never written to Niro's database.

So a deleted message is only logged if Niro saw it in the last 7 days while logging was on. Older messages, and messages from ignored channels, roles or bots, are not logged.

To turn this off, tick **Don't log message text** under **Privacy**. Niro then never keeps or shows what messages said: edits and deletions are logged without their text.

## Ignore lists

Under **Ignore**:

- **Ignore bots** (on by default): skip messages and voice activity of bots and webhooks. Bots joining or leaving are always logged.
- **Ignored channels**: messages and voice activity in these channels are not logged.
- **Ignored roles**: messages and voice activity of members with these roles are not logged.

Ignore lists apply to message and voice logs only.

## Permissions

Niro needs to see each log channel, send messages and attach files there. Moderation and server changes come from the audit log (View Audit Log). Showing which invite a member used needs Manage Server. Both are in the permissions Niro asks for when you add it.

## During a flood

Each log channel has a queue of up to 200 entries, posted in order. In a very large flood, entries beyond that are dropped.
