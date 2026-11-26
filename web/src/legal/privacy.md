# Privacy Policy

Draft for review, version 0.1 (2026-11-26). Applies to the hosted Niro bot and dashboard at niro.nithron.com.

## Who we are

Niro is run by Nithron [TO FILL: legal form], [TO FILL: registered address], Belgium, enterprise number [TO FILL: KBO/BCE number] ("Nithron", "we"). Nithron is the data controller for the personal data described here.

Contact for anything about your data: privacy@nithron.com.

People who run their own copy of Niro (self-hosting) are the controllers of their own copy. This policy does not cover them.

## What Niro does

Niro is a Discord bot for moderation, protection against scams and raids, server logs, role panels, welcome messages and support tickets. A server's admins add Niro and choose which features to turn on in the dashboard. Niro only sees what Discord sends to bots in servers it has been added to.

## What we process, why, and for how long

| Data | Why | Kept |
|---|---|---|
| **Dashboard login:** your Discord user ID, username, display name, avatar, the list of servers you can manage, and a Discord access token | To sign you in and show the servers you can configure | The session ends after 24 hours without use, after 7 days at most, or when you log out. "Log out on all devices" ends every session at once. The token is never sent to your browser, and the session's token is revoked at Discord when you log out |
| **Server settings:** the server's ID and name, its settings, and who changed which setting and when | To run the features the server's admins turned on | While Niro is in the server. The change history is kept for 1 year |
| **Moderation cases:** the member's user ID and name, the moderator, the action, the reason and the duration | Moderation records for the server's staff | While Niro is in the server |
| **Guardian events:** user and channel IDs, counts, hashes and the type of detection. Never the text of messages | To detect hacked accounts, scam links and raids, and show staff what happened | 90 days. Events a moderator marked "kept", and quarantines still waiting for a decision, stay until resolved or until Niro leaves the server |
| **Server log message cache:** the text and author of recent messages | Only if the server log is on, to show what an edited or deleted message said | 7 days, in memory storage, never in the database. Admins can turn this off |
| **Ticket transcripts:** the messages in a closed ticket (author names and text) and attachment file names | A record of support conversations for the server's staff | 90 days by default. The server's admins can set between 1 and 365 days |
| **Role panels, welcome settings, ticket panels:** the messages and roles the admins configured | To run those features | While Niro is in the server |
| **Technical data:** IP addresses of dashboard visitors, and service logs (which name user and server IDs, never message text) | Security, rate limiting (stopping abuse) and fixing faults | IP-based rate-limit counters last 1 minute. Service logs are rotated out once they reach 50 MB per service, usually within days |
| **Backups** of the database | To recover from failures | 28 days at most. Every copy is encrypted |

Messages are read by Niro as they arrive, to check for scams and spam. Apart from the cache and transcripts above, message text is not stored.

## When Niro leaves a server

If Niro is removed from a server, all of that server's data is deleted after 30 days. Adding Niro back within those 30 days restores it.

A server's owner can also delete everything at once from the dashboard ("Delete data…" on the server's page). Niro then leaves the server, and its data is deleted immediately. Copies in backups disappear within 28 days.

## Legal basis

- **Running the features a server's admins chose:** our legitimate interest, and theirs, in a safe, moderated community (GDPR art. 6(1)(f)). Niro collects only what each feature needs. Moderation records and evidence are limited and expire as listed above.
- **Dashboard login:** necessary to provide the dashboard you ask to use (art. 6(1)(b)).
- **Security, abuse prevention and backups:** legitimate interest (art. 6(1)(f)).
- **Legal obligations,** where they apply (art. 6(1)(c)).

## Who else processes data

- **Discord** delivers the events Niro receives and stores the messages Niro posts. Discord is a separate controller under its own [Privacy Policy](https://discord.com/privacy).
- **Our hosting provider,** [TO FILL: provider name and data-centre country], runs the server Niro runs on.
- **Our backup storage provider,** [TO FILL: provider name and country], stores encrypted backup copies.

We don't sell personal data, use it for advertising, or train AI models on it.

Data is stored in [TO FILL: country, ideally in the EU]. Discord is based in the United States. Transfers to Discord follow Discord's own safeguards.

## Your rights

You can ask us for:

- a copy of your data (access);
- correction of your data (rectification);
- deletion of your data (erasure);
- a restriction of processing;
- your data in a portable format;
- and you can object to processing based on legitimate interest.

Write to privacy@nithron.com from the Discord account the request is about, or tell us your Discord user ID and how to confirm it's you. We answer within one month.

Some requests are about moderation records a server's staff keep, such as a ban reason. We may ask that server's admins first, and we weigh your request against the server's interest in keeping its records.

You can also complain to the Belgian Data Protection Authority (Gegevensbeschermingsautoriteit / Autorité de protection des données, www.dataprotectionauthority.be), or to the authority where you live.

## Children

Niro is a tool for Discord servers. Discord requires its users to be at least 13 (older in some countries). We don't knowingly collect data from anyone below Discord's minimum age.

## Security

- **Encryption:** connections use HTTPS, and backups are encrypted.
- **Access:** the dashboard checks your rights on Discord for every server action, and access to the servers is limited to Nithron staff who run the service.
- **Open code:** Niro's code is open for review at github.com/Nithron/Niro. Please report security problems as described in its SECURITY.md.

## Changes

We'll announce changes in Niro's support server and update the date above. If a change affects how we use your data, we'll say so before it takes effect.
