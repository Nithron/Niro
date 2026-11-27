# Security policy

Niro is a security bot, so its own security comes first. If you think you've found a vulnerability,
**do not open a public issue**.

## Reporting a vulnerability

Report it privately, either way:

- GitHub: **Security → Report a vulnerability** on this repository (private vulnerability reporting), or
- email **security@nithron.com**.

Please include what is affected (bot, API, dashboard, a specific command or endpoint), the impact, steps to
reproduce, and the version or commit. I acknowledge reports within 5 business days.

## What I most want to hear about

- Ways to make Niro act with its permissions on someone's behalf: bypassing role-hierarchy or permission
  checks, acting in a server you don't manage, or abusing a dashboard endpoint (authorization, CSRF, sessions).
- Ways to make Niro leak data: message text, cases, Guardian evidence, or another server's settings.
- Bypasses of Guardian that are reliable and general (not one missing scam domain: those are regular issues).
- Making log entries or alerts ping people, or spoof Niro's own messages.

## Disclosure

- Nithron follows coordinated disclosure with a **90-day** default embargo; critical issues are fixed faster.
- Reporters are credited in the release notes if they want.

## Scope

- In scope: the code in this repository and the official hosted Niro bot.
- Out of scope: third-party dependencies (report upstream), forks and self-modified builds, Discord itself,
  denial of service by volume, and social engineering.

## Supported versions

Niro is in alpha (`v0.1.0-alpha.x`). Only the latest release and `main` get security fixes.
