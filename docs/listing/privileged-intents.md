# Privileged intents: application answers

Use these answers for the production application "Niro" (Developer Portal → App Verification). They describe the code as of `v0.1.0`. Change them if the code changes.

Niro requests **Server Members** and **Message Content**. It never requests **Presence**.

## What does your application do?

Niro is a security-first moderation bot for Discord communities, with a web dashboard. Its features:

- moderation commands and case records;
- protection against hacked accounts, scam links and raids ("Guardian");
- a server log;
- role panels;
- welcome and goodbye messages;
- support tickets.

Server admins turn each feature on separately. Niro asks for least-privilege permissions, never Administrator. Its code is published for review at https://github.com/Nithron/Niro.

## Server Members intent

**Why it is needed:**

- **Raid protection:** Guardian counts member joins in a short window to detect raids, and switches on raid mode (pausing invites, raising the verification level). That needs `GUILD_MEMBER_ADD`.
- **Account-age gate:** new members whose accounts are younger than the server's minimum age are held back or removed, as the server chooses.
- **Welcome and goodbye:** welcome messages, roles given on join, and goodbye messages need member join and leave events.
- **Server log:** the members log records joins, leaves and nickname or role changes, for moderators.

**How the data is used and stored:**

- Join events are counted in memory (Valkey) for a few minutes, as anonymous counters per server.
- When Guardian acts, it stores an event with the member's user ID and the reason. That event expires after 90 days.
- Welcome messages are posted and not stored.
- Server log entries are posted to the server's own log channel and not stored by Niro.
- No member data is shared with third parties, sold, or used for anything other than the server it comes from.

**Without it:** raid detection, the account-age gate, welcome messages and the members log would not work. The bot connects without the intent and turns those features off (`privilegedIntents` in `internal/platform/discord/bot.go`).

## Message Content intent

**Why it is needed:**

- **Scam links:** Guardian checks links in messages against a list of phishing domains and look-alike domains (for example `dlscord-gift.com`). That needs the message text.
- **Hacked-account spam:** a compromised account usually posts the same message in many channels within seconds. Guardian compares a hash of normalised message text across channels to spot this, then quarantines the account so moderators can review it.
- **Server log:** when a message is edited or deleted, the log shows what it said, so moderators can act on content that was removed to hide abuse.
- **Ticket transcripts:** when a support ticket closes, its messages are saved as a transcript for the server's staff.

**How the data is used and stored:**

- Message text is read when it arrives and is not stored in Niro's database, with one exception: ticket transcripts.
- **Guardian** keeps only hashes, counts and channel IDs, for minutes. Its events store which rule fired, never the text.
- **Server log:** to show edited and deleted text, recent messages are cached in memory (Valkey) for at most 7 days. Admins can turn this off per server ("Don't log message text"). The cache is removed when a message is deleted and logged.
- **Ticket transcripts** are compressed and deleted after 90 days by default. Admins can set 1 to 365 days.
- When Niro is removed from a server, all of that server's data is deleted after 30 days. The owner can delete it at once from the dashboard.
- Message content is never used to train models, never shared and never sold.

**Without it:** scam-link protection and hacked-account detection would not work, and the server log could not show what deleted or edited messages said. Discord's native AutoMod cannot catch cross-channel spam from one compromised account, or link look-alikes.

## Screenshots and video to attach

1. A Guardian alert in a log channel after a scam link was posted. Use the test server; make the alert with the demo steps in `docs/milestones/m2-demo.md`.
2. The server log showing an edited and a deleted message (`m3-demo.md`).
3. A welcome message, and a raid-mode summary (`m4-demo.md`, `m2-demo.md`).
4. The dashboard's logging settings with "Don't log message text".

## Links

- Privacy Policy: https://niro.nithron.com/privacy
- Terms of Service: https://niro.nithron.com/terms
- Support server: [TO FILL: invite link]
