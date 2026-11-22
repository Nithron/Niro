# M2 demo checklist

**Done when** (plan §7): in the test server, a simulated hacked-account burst is quarantined within about 2 seconds with every copy deleted; a simulated raid triggers raid mode; and the false-positive review flow restores a user correctly.

## Setup

1. `make dev-deps`, then `make run` (applies migration 3 and downloads the scam-domain list on first start; the log shows `scam-domain feed updated … domains=21852`).
2. Niro Dev already has **Server Members** and **Message Content** switched on in the Developer Portal. If either is off, the bot still starts but logs a warning saying which Guardian features are off.
3. A **second account** in the test server with a role **below** Niro Dev's role (as in M1). It must have joined more than a minute ago and **not** have Manage Messages or Timeout Members (moderators are never checked).
4. At least three text channels the second account can write in.
5. Dashboard → **Guardian**: pick an **Alert channel** (or leave "Use the moderation log channel" if M1's log channel is set), Save.

## Demo

| # | Do | Expect |
|---|---|---|
| 1 | Dashboard → **Security audit** (or `/guardian audit`) | Findings such as "@everyone has Mention @everyone", "verification level is None", "moderators don't need 2FA". Nothing is changed |
| 2 | As the second account, post `look at this https://example.org/a` in channel 1, then the same in channels 2 and 3, quickly (within a minute) | On the third copy: all three messages disappear, the member is timed out for 7 days, and an alert "Possible hacked account" appears with Restore / Ban / Keep quarantined. The member gets a DM with recovery steps. The moderation log shows a **Quarantine** case |
| 3 | Check the timing in the bot log: `guardian acted … took=…` | Well under 2 seconds |
| 4 | Press **Restore** on the alert | Timeout lifted, alert turns green "… restored the member", a **Quarantine lifted** case appears. Pressing a button again says another moderator already handled it |
| 5 | As the member, post `free nitro https://dlscord.gift/x` once | Quarantined on the first message (look-alike domain); alert lists "Look-alike domain (`dlscord.gift`)" and "Known scam wording" |
| 6 | Dashboard → **Detections**: press **Ban** on that event | Member banned (case in the log), the Discord alert updates too. Unban them afterwards with `/unban` |
| 7 | As the member, post `[discord.com/nitro](https://example.org)` | Message deleted (masked link), alert only; the member is **not** restricted |
| 8 | Post normal messages, the same GitHub link as another member, or "gm" in three channels | Nothing happens |
| 9 | Dashboard → Guardian: set Mode to **Monitor**, repeat step 2 | Alert says "Monitor mode: Guardian took no action"; messages stay, no timeout. Set Mode back to **Enforce** |
| 10 | **Raid:** Guardian settings → Joins **3** within **60** seconds, Save. Have three accounts join within a minute (or leave and rejoin with alts) | On the third join: "Raid detected" alert with **End raid mode**; Server Settings → Safety shows invites paused; verification level is **High** |
| 11 | Press **End raid mode** (or `/guardian raid end`) | Invites resume and the verification level returns to what it was. Set Joins back to 10 |
| 12 | `/guardian raid start`, then `/guardian raid status`, then end it from the dashboard's Detections page | Same as 10–11, started by hand |
| 13 | `/lockdown channel reason:test` in a channel | A 🔒 notice; members can't send there. `/unlock channel` restores the channel's exact previous permissions |
| 14 | `/lockdown server`, then `/unlock server` | Every text channel locked, then restored |
| 15 | Verification gate: Guardian settings → Verified role = a test role, min age 0, Save. `/guardian verify-panel channel:#welcome`, press **Verify** as the member | The member gets the role; pressing again says "already verified" |
| 16 | Account-age gate: set Minimum account age to **8760** hours (one year) with action **Only alert**, then have an account younger than a year join | A "New account" alert; the member stays. Set it back to 0 |

## Notes

- **Privacy:** Guardian reads messages in memory only. Postgres stores a summary (which channels, how many messages, hashes, domains), never message text. Recent message references live in Valkey for at most the burst window plus a minute.
- **Scam domains:** the default list is [nikolaischunk/discord-phishing-links](https://github.com/nikolaischunk/discord-phishing-links) (MIT, ~21.9k domains, last updated March 2024), refreshed every 6 hours. Look-alike checks catch new domains the list doesn't have. Self-hosters can set `NIRO_SCAM_FEEDS` or turn it off. Niro never opens links users post.
- **Permissions:** raid mode needs Manage Server (pausing invites, verification level) and Manage Channels (slowmode); `/lockdown` needs Manage Channels and Manage Roles. All are in the invite already. Listing webhooks in the audit would need Manage Webhooks, which Niro deliberately doesn't request; the audit says so instead.
- **Quarantine role mode** only restricts members if that role's channel permissions are set up (e.g. it can see only one help channel). Timeout mode works with no setup and is the default.
