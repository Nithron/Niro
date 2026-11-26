![Niro](assets/brand/png/social/readme-banner.png)

# Niro

A security-first Discord bot by [Nithron](https://nithron.com): one bot for moderation, scam and raid
protection, logging, roles, welcome and tickets, managed from a web dashboard. AI features
powered by nAI and NithronOS integration will follow.

> **Status: alpha (`v0.1.0-alpha.1`).** Moderation, Guardian protection and server logging work end to end.
> Role panels, welcome and tickets (M4) are built and waiting for their demo; the self-host guide and hosted
> launch (M5) come next. See the [build plan](docs/plan.md). Expect breaking changes before `v0.1.0`.

## What works today

- **Moderation:** `/warn`, `/timeout`, `/kick`, `/ban` (with confirmation and temporary bans), `/purge`, numbered
  cases with a moderation log, and Discord's native AutoMod managed from the dashboard.
- **Guardian:** hacked-account bursts and scam links are caught and the member quarantined, with a
  Restore / Ban / Keep review flow; anti-raid mode, `/lockdown`, a verification gate and a permission audit.
  Niro never opens links users post.
- **Logging:** message edits and deletes (bulk deletes as a file), joins with the invite used, member,
  moderation, server, voice, AutoMod and Guardian events, routed per category.
- **Dashboard:** Discord login and every setting above, applied live without a restart.
- **Privacy:** least-privilege permissions (never Administrator), no message text in the database, and
  servers can turn message-text logging off.

Self-hosting works from source today ([dev setup](docs/dev-setup.md)); the supported self-host guide and
container images arrive with `v0.1.0`.

## Docs

- [Build plan](docs/plan.md) · [Decisions log](docs/decisions.md) · [Changelog](CHANGELOG.md)
- [Dev setup](docs/dev-setup.md) · [Contributing](CONTRIBUTING.md) · [Security policy](SECURITY.md)
- Research: [discovery](docs/research/2026-11-discovery.md), [Go library check](docs/research/2026-11-go-library-check.md)
- [Brand assets](assets/brand/README.md)

## License

Niro's source code is available under the [Nithron Community License (NCL) v1.0](LICENSE): free for
non-commercial use. Commercial use, hosting for others, or redistributing binaries needs a commercial
license; see [COMMERCIAL.md](COMMERCIAL.md). “Niro” and its logos are trademarks of Nithron; see
[TRADEMARK_POLICY.md](TRADEMARK_POLICY.md).

Niro is not affiliated with or endorsed by Discord Inc.
