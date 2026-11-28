# Contributing to Niro

Thanks for your interest! Issues, ideas, docs and code are all welcome.

## Ground rules

- Be respectful and constructive.
- Security problems: follow [SECURITY.md](SECURITY.md), never a public issue.
- Contributions are made under the [Nithron Community License (NCL)](LICENSE) and its contribution clause (§4).
  No separate CLA or sign-off is needed.
- Niro is **source-available**, not OSI open source; see [COMMERCIAL.md](COMMERCIAL.md) for commercial use.

## Before you start

Open an issue before large changes, so the approach can be agreed first. The [build plan](docs/plan.md) shows what
is planned per release, and the [decisions log](docs/decisions.md) why things are the way they are.

## Development

Setup, the Discord dev application and `.env`: [docs/dev-setup.md](docs/dev-setup.md). In short:

```
make dev-deps   # Postgres + Valkey in Docker
make run        # dashboard build, migrations, bot + API on http://localhost:8080
```

## Checks (all run in CI)

```
make lint            # golangci-lint, ESLint, TypeScript
make test            # Go tests, including Postgres/Valkey integration tests (needs Docker)
make generate-check  # sqlc, OpenAPI server and TypeScript client are up to date
make contract-lint   # OpenAPI lint and sqlc vet
cd web && pnpm test  # dashboard unit tests
make e2e             # dashboard Playwright suite (pnpm exec playwright install chromium once)
```

## How the code is laid out

- `internal/modules/*`: platform-agnostic features (moderation, guardian, logging). They never import disgo.
- `internal/platform/discord`: the Discord adapter (events in, actions out, Components V2 cards).
- `internal/store` (Postgres: sqlc queries, goose migrations), `internal/cache` (Valkey), `internal/api`
  (spec-first HTTP API from `api/openapi.yaml`), `web/` (React dashboard embedded in the binary).

## Conventions

- **Tests first for domain logic.** Guardian heuristics have a replay harness
  (`internal/modules/guardian/testdata`): add a scenario for every false positive or miss you fix.
- **Privacy:** never store message text in Postgres; keep it in Valkey with a TTL, or not at all.
- **Least privilege:** don't add Discord permissions or intents without a reason in the pull request.
- **Every user-facing string** goes through the catalogs (`internal/i18n/locales/en.toml`,
  `web/src/locales/en/common.json`); tests fail on missing keys.
- **Messages never ping:** send with no allowed mentions and escape user text.
- After editing SQL queries or `api/openapi.yaml`, run `make generate` and commit the result.
- Small, focused pull requests with a clear description of what changed, why, and how you tested it.
