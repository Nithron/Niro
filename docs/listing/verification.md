# Verification and launch checklist

An unverified bot can join at most 100 servers. Discord accepts verification requests once a bot is in **75** servers, so the hosted bot first goes live as a soft launch. Privileged intents for verified bots need approval (see [privileged-intents.md](privileged-intents.md)).

## Before the soft launch

- [ ] The hosted deployment is up ([runbook](../ops/runbook.md)), alerts reach `#ops-alerts`, and the off-site copy ran once.
- [ ] The Terms and Privacy Policy are reviewed, the `[TO FILL: …]` fields filled in, and the pages are live at `/terms` and `/privacy`.
- [ ] The support server is set up ([support-server.md](support-server.md)) and its invite is filled in where the docs say `[TO FILL: invite link]`.
- [ ] Developer Portal (Niro) → General Information has the Terms and Privacy URLs, the description, the icon and the banner.
- [ ] Developer Portal (Niro) → OAuth2 has the redirect `https://niro.nithron.com/auth/callback`.
- [ ] Developer Portal (Niro) → Bot has Server Members and Message Content turned on (allowed below 100 servers).

## Verification (from 75 servers)

- [ ] The team owner's identity is verified (Developer Portal → Team → verification through Discord's identity provider).
- [ ] Apply in Developer Portal → App Verification:
  - Answer the data questions with [privileged-intents.md](privileged-intents.md).
  - Attach the screenshots it lists.
  - Link the Privacy Policy.
- [ ] While the review runs, the bot stays at or below 100 servers. Discord enforces this.

## After approval

- [ ] Fill in the App Directory listing ([app-directory.md](app-directory.md)) and publish it.
- [ ] Tag `v0.1.0`, publish the release notes, and post in `#announcements`.
