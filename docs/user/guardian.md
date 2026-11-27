# Guardian

Guardian protects your server against hacked accounts, scam links and raids, and checks your server for dangerous permissions. It is on by default.

Server owners and members who can manage messages are always trusted and never checked. You can add more trusted roles under **Trusted roles**.

## First setup

Dashboard > your server > **Guardian**:

1. Pick an **Alert channel** that only moderators can see, or keep **Use the moderation log channel**.
2. Choose a **Mode**. **Enforce** (the default) deletes, quarantines and starts raid mode. **Monitor** only posts alerts, so you can see what Guardian would do before it acts.
3. Save. Changes apply immediately.

## What Guardian detects

| Signal (as shown in alerts) | Example |
|---|---|
| Same message in several channels | A hacked account posts one link everywhere. Default: 3 channels within 60 seconds |
| Known scam domain | The link is on the public scam-domain list Niro uses |
| Domain on your block list | You listed the domain under **Always block these domains** |
| Look-alike domain | `dlscord.gift` pretending to be Discord, or a fake Steam site |
| Link text shows a different site | `[discord.com/nitro](https://example.org)` |
| Known scam wording | "Free Nitro", fake giveaways, Steam gifts |
| Long-silent member posting links | Silent for 30 days (by default), then posts links. Never enough on its own |

Niro never opens links that members post. Domains under **Always allow these domains** are never flagged (subdomains included).

## How Guardian responds

Guardian adds up the signals on a message:

- **Strong signals** (a scam, block-listed or look-alike domain, or the same link in several channels): Niro deletes every copy, quarantines the member, sends them a DM with recovery steps, and posts an alert.
- **A link whose text shows a different site**: the message is deleted and an alert is posted. The member is not restricted.
- **Weak signals**, such as the same plain text in several channels: an alert only.

In Monitor mode, alerts say "Monitor mode: Guardian took no action."

## Quarantine

Under **Quarantine**, choose **How to restrict**:

- **Time out** (default): works with no setup. **Timeout length (hours)** defaults to 168 (7 days); the maximum is 28 days.
- **Replace their roles with a quarantine role**: their roles are saved and given back on Restore. You must set up that role's channel permissions yourself, for example so it only sees one help channel.

**DM the member recovery steps** is on by default. The DM explains how to secure a hacked account and to contact your moderators.

Each alert has three buttons:

- **Restore**: lifts the restriction and gives back any saved roles.
- **Ban**: bans the member and deletes their messages from the last day.
- **Keep quarantined**: leaves the restriction in place.

Restore and Ban create cases in the moderation log. Once one moderator has handled an alert, the buttons say so to others.

The **Detections** tab lists everything Guardian found and did, newest first, with the same buttons. Use **Show** to filter by **Waiting for review** or **Kept in quarantine**.

## Raids

Under **Anti-raid**, Guardian starts raid mode when too many members join at once (default: 10 **Joins** within 30 seconds). Raid mode:

- pauses invites (**Pause invites during raid mode**);
- raises the verification level to High (**Raise the verification level to High during raid mode**);
- optionally sets slowmode (**Slowmode during raid mode (seconds)**, 0 leaves it alone);
- lasts 30 minutes by default (**Raid mode lasts (minutes)**).

Everything raid mode changes is undone when it ends. The alert has an **End raid mode** button. You can also use the **Detections** tab (**Start raid mode** / **End raid mode**) or these commands:

| Command | What it does |
|---|---|
| `/guardian raid start` | Turn raid mode on now |
| `/guardian raid end` | Turn it off and undo its changes |
| `/guardian raid status` | Show whether raid mode is on |

## Account-age gate

**Minimum account age (hours)**: accounts younger than this when they join get the **Action** you choose: **Only alert**, **Kick** or **Quarantine**. 0 (the default) turns the gate off.

## Verification gate

1. Under **Verification gate**, pick a **Verified role**. Optionally set **Minimum account age to verify (hours)**.
2. Run `/guardian verify-panel channel:#your-channel`. Niro posts a message with a **Verify** button.
3. Set your channel permissions so the rest of the server needs the verified role.

Members who press **Verify** get the role. Accounts that are too new are told when they can try again. If you use [Welcome](welcome.md) roles on join, they are given on Verify instead of on join.

## Lockdown

| Command | What it does |
|---|---|
| `/lockdown channel [channel] [reason]` | Stop members sending messages in one channel (this one by default) |
| `/lockdown server [reason]` | Lock every text channel |
| `/unlock channel [channel]` | Unlock one channel |
| `/unlock server` | Unlock every channel Niro locked |

Unlocking restores each channel's exact previous permissions. Niro needs Manage Channels and Manage Roles in those channels.

## Security audit

Run `/guardian audit` or open the **Security audit** tab. It is read-only: nothing is changed. It flags, by severity:

- `@everyone` with dangerous permissions;
- roles and bots with Administrator or other dangerous permissions, with member counts;
- Niro itself having Administrator (it doesn't need it);
- a verification level of None or Low;
- moderators not being required to use two-factor authentication.

Webhooks are not checked, because Niro doesn't ask for Manage Webhooks.

`/guardian` commands need Manage Server; `/lockdown` and `/unlock` need Manage Channels.

## What Guardian stores

Guardian reads messages in memory only. For each detection it stores a summary: which channels, how many messages, hashes, domains and what it did. **Never message text.** Summaries are deleted after 90 days, except ones still waiting for review, kept in quarantine, or for a raid or lockdown still in effect. See [Privacy and data](privacy-and-data.md).
