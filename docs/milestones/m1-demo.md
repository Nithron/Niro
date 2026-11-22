# M1 demo checklist

**Done when** (plan §7): the full case lifecycle works from commands and the dashboard, and AutoMod rules created in the dashboard appear in Discord's own AutoMod settings.

## Setup

1. `make dev-deps`, then `make run` (applies migration 2 automatically).
2. You need a second account (or a friend) in the test server as the member to moderate. Give it a role **below** Niro Dev's role.
3. Make sure Niro Dev's role is above that member's highest role (Server Settings → Roles).

## Demo

| # | Do | Expect |
|---|---|---|
| 1 | Dashboard → your server → **Moderation** tab: choose a log channel, Save | "Saved" |
| 2 | `/warn user:<member> reason:test` | Private reply "Case #1 · Warned …", the member gets a DM, a case card appears in the log channel |
| 3 | `/timeout user:<member> duration:10m` | Member is timed out for 10 minutes; case #2 in the log with an "ends in 10 minutes" time |
| 4 | `/untimeout user:<member>` | Timeout lifted; case #3 |
| 5 | `/warn user:<yourself>` or a user above you | A clear refusal ("You can't do that to yourself" / "Your highest role must be above …"); no case created |
| 6 | `/ban user:<member> duration:2m reason:test` → **Ban** | Confirmation first; after confirming: DM sent, member banned, case #4 "temporary" |
| 7 | Wait about 2 minutes | Niro lifts the ban by itself (case #5, unban "automatic"), and the member can be re-invited |
| 8 | Post a few messages, some with links, then `/purge count:10 links:true` | Only the messages with links are deleted; purge case in the log |
| 9 | `/history user:<member>` | All cases above, plus a "View all in the dashboard" link |
| 10 | `/case reason number:1 reason:edited` | The log message for case #1 updates in place |
| 11 | Dashboard → **Cases**: filter by "Ban", open a case, edit its reason, then delete another case | List and detail update; the deleted case's log message is struck through as "deleted" |
| 12 | Dashboard → **AutoMod** → Add **Block invite links** | Server Settings → AutoMod in Discord shows "Niro · Block invite links" |
| 13 | As the member, post `discord.gg/test` | Discord blocks it with "Invite links aren't allowed here." |
| 14 | Dashboard: turn the rule off, then delete it | Reflected in Discord's AutoMod settings |
| 15 | Modules tab: turn **Moderation** off, run `/warn` | "The Moderation module is turned off …" |

## Notes

- `/purge` with `links:true` reads message text, so the Dev app needs the **Message Content** intent switched on in the Developer Portal (it already is, per dev-setup). No new gateway intents were added in M1.
- Commands default to the matching Discord permission (Moderate Members, Kick, Ban, Manage Messages). Server admins can change who sees them under Server Settings → Integrations → Niro Dev.
