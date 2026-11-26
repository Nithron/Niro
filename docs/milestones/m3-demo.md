# M3 demo checklist

**Done when** (plan §7): every event type produces a correct log entry in the test server.

## Setup

1. `make dev-deps`, then `make run`. No new migration: log entries are posted to Discord, and message text lives only in Valkey for 7 days.
2. Niro Dev needs no new privileged intent. M3 adds four ordinary ones (Guild Moderation, Guild Voice States, Guild Invites, AutoMod Execution), which Discord grants automatically.
3. In the test server, create `#logs` and `#mod-logs` (Niro must be able to view them, send messages and attach files). Keep the **second account** from M1/M2 (a role below Niro's, no moderator permissions).
4. Dashboard → **Modules**: turn **Logging** on. Dashboard → **Logging**: Default log channel = `#logs`, Moderation = `#mod-logs`, turn **Voice joins / leaves / moves** on, Save.
5. Leave M1's moderation log channel set to something other than `#mod-logs` for steps 9–12 (step 13 covers the case where they are the same).

## Demo

| # | Do | Expect in the log channel |
|---|---|---|
| 1 | As the second account, post `hello wrld`, then edit it to `hello world` | **Message edited** in `#logs`: author, channel, jump link, *Before* `hello wrld`, *After* `hello world` |
| 2 | Delete that message | **Message deleted** with the author and `hello world` |
| 3 | Post three messages, then `/purge count:3` | One **Messages bulk-deleted** entry, "By: Niro (/purge)", with a `.txt` file listing the three messages (time, author, text) |
| 4 | Have an account join with an invite you created (Server Settings → Invites shows its uses) | **Member joined**: account age, "⚠️ less than a week old" for new accounts, the invite code and who created it |
| 5 | That account leaves | **Member left** with when it joined and its roles |
| 6 | Change the second account's nickname | **Nickname changed** old → new, and who changed it |
| 7 | Give the second account a role, then remove it | Two **Roles changed** entries (Added / Removed) with who did it |
| 8 | Create a text channel, rename it, change its slowmode, drag it to another position, then delete it | **Channel created**, **Channel changed** (Name, Slowmode, Position), **Channel deleted**, each with who did it |
| 9 | Create a role, change its colour and permissions, move it in the role list, delete it | **Role created / changed / deleted**; permissions show by name; the position change is logged |
| 10 | Change the server's verification level | **Server settings changed**: Verification level None → Low (set it back afterwards) |
| 11 | `/timeout` the second account for 1 minute, then `/untimeout` | Two **Timeout** entries in `#mod-logs` ("Until …", then "The timeout was removed"), By: Niro, with the reason |
| 12 | Kick the second account from Discord's own member list (right-click → Kick), then re-invite; ban and unban it with `/ban` / `/unban` | **Member kicked** (By: you), **Member banned**, **Member unbanned** in `#mod-logs`. The ban and kick also show **Member left** in `#logs` |
| 13 | Dashboard → Logging: set Moderation to M1's moderation log channel, Save. `/timeout` the account again | No extra log entry there: the case card is already in that channel. A timeout given from Discord's own UI *is* logged there. Set Moderation back to `#mod-logs` |
| 14 | Join a voice channel, move to another, leave | **Joined voice**, **Moved voice channel**, **Left voice** |
| 15 | Dashboard → AutoMod: add the "invite links" preset. As the second account, post `discord.gg/abc` | One **AutoMod acted** entry (rule, matched keyword, "Message blocked", the text), not one per action |
| 16 | Trigger Guardian as in M2 step 5 (`free nitro https://dlscord.gift/x`) with Guardian's alert channel ≠ `#logs` | **Guardian acted** in `#logs` (detection, action, signals) as well as the usual alert with buttons; the deleted scam message shows "By: Niro (Guardian)" |
| 17 | Dashboard → Logging: turn **Don't log message text** on, Save. Post, edit and delete a message | Edit and delete are logged with "Message content logging is off", no text |
| 18 | Add `#general` to **Ignored channels** (or the second account's role to **Ignored roles**), Save. Post, edit and delete there | Nothing is logged |
| 19 | Delete one of Niro's own log entries in `#logs` | Nothing is logged (log channels are never logged) |

## Notes

- **Privacy:** message text is cached in Valkey for at most 7 days and removed as soon as the message is deleted, even when logging has been turned off since. It is never written to Postgres. With "Don't log message text" on, no text or file names are kept at all.
- **Deleted messages that weren't cached** (older than 7 days, sent before logging was on, or from ignored bots and roles) are not logged: there is nothing to show about them.
- **Who did it** comes from Discord's audit log, which Niro receives live (View Audit Log is already in the invite). Discord doesn't say who deleted a single message, so deletes only name Niro when Niro did it.
- **Invite tracking** compares invite use counts before and after the join and needs Manage Server (already in the invite). It shows "unknown" rather than guess when two people join at once or a vanity URL is used.
- **Rate limits:** each log channel has its own queue of up to 200 entries, sent in order. During a flood, older entries wait and overflow is dropped and counted (`niro_log_entries_total{result="dropped"}`).
