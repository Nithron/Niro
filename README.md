![Niro](assets/brand/png/social/readme-banner.png)

# Niro

A security-first Discord bot by [Nithron](https://nithron.com): one bot for moderation, scam and raid
protection, logging, roles, welcome and tickets, managed from a web dashboard. AI features
powered by nAI and nOS integration will follow.

> **Status: alpha, heading for `v0.1.0`.** Every v0.1 feature works end to end. The last milestone (M5:
> hardening, self-hosting, the hosted launch) is in progress. See the [build plan](docs/plan.md).

## What works today

- **Moderation:** `/warn`, `/timeout`, `/kick`, `/ban` (with confirmation and temporary bans), `/purge`, numbered
  cases with a moderation log, and Discord's native AutoMod managed from the dashboard.
- **Guardian:** hacked-account bursts and scam links are caught and the member quarantined, with a
  Restore / Ban / Keep review flow; anti-raid mode, `/lockdown`, a verification gate and a permission audit.
  Niro never opens links users post.
- **Logging:** message edits and deletes (bulk deletes as a file), joins with the invite used, member,
  moderation, server, voice, AutoMod and Guardian events, routed per category.
- **Community:** role panels, welcome and goodbye messages with auto-roles, and support tickets with
  private threads and transcripts, all designed with a shared message builder.
- **Dashboard:** Discord login and every setting above, applied live without a restart.
- **Privacy:** least-privilege permissions (never Administrator), no message text in the database except
  expiring ticket transcripts, servers can turn message-text logging off, data is deleted 30 days after
  Niro is removed, and owners can delete their server's data at once.

Run your own: the [self-host guide](docs/self-host.md) sets up Niro with Docker Compose, HTTPS and nightly
backups. Niro is built from source; there is no prebuilt image.

## Docs

- [User guide](docs/user/README.md) · [Self-host guide](docs/self-host.md)
- [Build plan](docs/plan.md) · [Decisions log](docs/decisions.md) · [Changelog](CHANGELOG.md)
- Operations: [runbook](docs/ops/runbook.md), [backup and restore](docs/ops/backup-restore.md), [load-test baselines](docs/ops/baselines.md)
- [Dev setup](docs/dev-setup.md) · [Contributing](CONTRIBUTING.md) · [Security policy](SECURITY.md)
- Research: [discovery](docs/research/2026-11-discovery.md), [Go library check](docs/research/2026-11-go-library-check.md)
- [Brand assets](assets/brand/README.md)

## License

Niro's source code is available under the [Nithron Community License (NCL) v1.0](LICENSE): free for
non-commercial use. Commercial use, hosting for others, or redistributing binaries needs a commercial
license; see [COMMERCIAL.md](COMMERCIAL.md). “Niro” and its logos are trademarks of Nithron; see
[TRADEMARK_POLICY.md](TRADEMARK_POLICY.md).

Niro is not affiliated with or endorsed by Discord Inc.
