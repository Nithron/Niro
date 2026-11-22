# Changelog

All notable changes to Nithro. Versions follow [Semantic Versioning](https://semver.org/).

## Unreleased

### M0: Foundations

- **Bot:** connects with automatic sharding; `/ping`, `/about` and `/modules` (Manage Server only), built with Components V2 and Nithro's brand emojis (uploaded to the app automatically). Commands register instantly in a dev server or globally in production.
- **Modules:** Core, Moderation, Guardian, Logging, Role panels, Welcome and Tickets can be switched on and off per server. Their features arrive in M1–M4.
- **Live configuration:** changes made in the dashboard reach every bot process within about a second (Valkey pub/sub), with no restart. Every change is audited (who, what, from where).
- **Dashboard:** Discord login, a list of the servers you can manage (with an "Add Nithro" invite link), and module switches that save instantly and roll back if saving fails. Dark and light themes follow your system.
- **Security:** least-privilege invite (no Administrator), server-side sessions with hashed IDs, OAuth state check, cross-site request blocking, strict security headers. The bot token never reaches the API process.
- **Languages:** English, with every user-facing string in translation catalogs (bot and dashboard) so more languages can be added without code changes.
- **Operations:** one `nithro` binary (`all | bot | api | migrate | version`), `/healthz`, `/readyz` and Prometheus `/metrics`, graceful shutdown, a 51 MB distroless container image, and CI with lint, tests (including Postgres/Valkey integration tests), a generated-code check and an image build.
