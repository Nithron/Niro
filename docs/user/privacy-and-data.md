# Privacy and data

This page explains what the hosted Niro (https://niro.nithron.com) stores about your server and for how long. The full [Privacy Policy](https://niro.nithron.com/privacy) is the binding text.

Nithron (Belgium) runs the hosted Niro and is responsible for its data. Questions or requests: privacy@nithron.com.

If you use a self-hosted copy of Niro, whoever runs it is responsible for its data. See the [self-host guide](../self-host.md#data-and-privacy).

## The short version

- Niro does not store message text in its database, with one exception: **ticket transcripts**.
- Message text for the server log lives in a short-lived cache for at most **7 days**.
- When Niro is removed from a server, all of that server's data is deleted after **30 days**.
- The server owner can delete everything at once from the dashboard.

## What Niro stores

| Data | What it contains | How long |
|---|---|---|
| Settings | Module settings, role and ticket panel definitions | While Niro is in the server |
| Settings change history | Which setting changed, and who changed it | 1 year |
| Moderation cases | Action, user, moderator, reason, duration, source | While Niro is in the server |
| Temporary ban timers | When to lift a ban | 30 days after the ban is lifted |
| Guardian detections | A summary: channels, message counts, hashes, domains, what Guardian did. **No message text** | 90 days. Detections still waiting for review, kept in quarantine, or for a raid or lockdown still in effect are kept until resolved, then deleted at the next cleanup |
| Ended raids | When they started and ended | 90 days after they end |
| Channel lockdowns | The channel's permissions before the lockdown, so they can be restored | Until the channel is unlocked |
| Recent message references for Guardian | Short-lived references used to spot the same message in many channels (cache, not the database) | The detection window plus a minute |
| Message cache for logs | Message text, so edits and deletions can be logged (cache, not the database) | Up to 7 days. Removed as soon as the message is deleted. Nothing is kept if **Don't log message text** is on |
| Tickets | Number, category, opener, status | While Niro is in the server |
| Ticket transcripts | The ticket's messages, as a compressed web page | 90 days by default; each server can choose 1 to 365 days |
| Dashboard login | Your Discord username, avatar and server list. Never your password or messages | For your login session |

**Log entries** are posted to your own Discord channels. Niro doesn't keep a copy; you decide how long they stay.

Deleting a case in the dashboard or with `/case delete` removes it from the history, but a record stays in the audit trail until Niro leaves the server.

## When Niro leaves your server

When Niro is removed from a server, all of that server's data (settings, cases, panels, tickets, transcripts and Guardian records) is deleted **30 days later**. If you add Niro back within those 30 days, everything is restored.

## Delete everything now

The server owner can delete all of a server's data at once:

1. Dashboard > your server > **Modules** tab.
2. At the bottom, under **Delete this server's data**, press **Delete data…**.
3. Type the server name to confirm, then press **Remove Niro and delete everything**.

This removes Niro from the server and deletes everything it stores about it right away. It cannot be undone. Only the server owner sees this option. If Niro was already removed, the same option is on the server's page in the dashboard.

## Backups

The hosted Niro keeps backups for at most 28 days. Data you delete, or that expires, is gone from backups within that time.

## Your members

Tell your members which Niro features you use, for example in your server rules. This matters most for logging (message text in the log cache) and tickets (transcripts).

A member who wants to know what Niro stores about them, or wants it deleted, can write to privacy@nithron.com.
