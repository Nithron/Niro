# M3 implementation plan: Logging, then `v0.1.0-alpha.1`

*2026-11-26. Built from plan §7 (M3), the M0–M2 code, and a check of disgo v0.20. The PM's answers to the open questions are recorded in [decisions](../decisions.md) #21.*

**Done when** (plan §7): every event type produces a correct log entry in the test server. Then tag `v0.1.0-alpha.1`, make the repo public, and invite early testers.

## 1. What already exists

- `core.ModuleLogging` is registered (off by default), so the module toggle, settings pipeline, live reload and config audit work today. M3 adds a `SettingsSpec` the same way Guardian does (`internal/modules/guardian/settings.go`, registered in `cmd/niro/main.go`).
- Patterns to copy:
  - Domain package with fakes and a JSON replay harness: `internal/modules/guardian`.
  - Typed Valkey helpers: `internal/cache/guardian.go`.
  - Listener to platform-agnostic event to `go b.safely(...)`: `guardian_commands.go` (`onGuildMessage`, `onMemberJoin`).
  - Components V2 cards posted to a channel: `guardian_present.go` (`PostAlert`) and `moderation_present.go` (`PostLog`).
  - Settings API with channel and role checks over RPC: `internal/api/guardian.go`.
  - Settings page: `web/src/pages/GuardianSettings.tsx`.
  - i18n completeness test with dynamic keys: `internal/i18n/i18n_test.go`.
- **View Audit Log** and **Manage Server** are already in the invite permissions (`internal/platform/discord/permissions.go`).
- **No migration is needed.** Log entries are posted to Discord and not stored. Message text lives only in Valkey with a TTL (plan §1, §5).

## 2. Design

### Package layout

```
internal/modules/logging/      types, settings, ignore rules, edit diff, bulk-delete file, service, fakes, replay testdata
internal/cache/logging.go      message cache (TTL, Take = read+delete), invite-use snapshot
internal/platform/discord/
  logging_listeners.go         gateway events -> logging events
  logging_platform.go          audit-log correlation, invite list
  logging_present.go           log cards (Components V2, no mentions)
  logging_queue.go             per-channel send queue
internal/api/logging.go        GET/PUT logging settings
web/src/pages/LoggingSettings.tsx
```

### Settings (`logging` module)

| Field | Default | Notes |
|---|---|---|
| `channels.{messages,members,moderation,server,voice,automod,guardian}` | unset | A destination per category; an unset category falls back to `default_channel_id`, and if that is unset too the category is off |
| `default_channel_id` | unset | |
| `events.<kind>` | all on except voice | Per event kind on/off; voice is noisy, so off by default |
| `ignore_channel_ids` (≤100), `ignore_role_ids` (≤25), `ignore_bots` (true) | | Bot joins and leaves are always logged, since they matter for security |
| `no_message_content` | false | When on, message text is neither cached nor shown. Edits and deletes are still logged, without content |

The message-cache TTL is the approved 7-day retention, set per deployment and not per server, so it can't be raised by a server setting.

### Events and intents

| Category | Events | Source (disgo v0.20) | Intent to add |
|---|---|---|---|
| messages | edit (diff), delete, bulk delete (file) | `GuildMessageUpdate`, `GuildMessageDelete`, see bulk note | already have Guild Messages + Message Content |
| members | join (account age, invite used), leave, nickname, roles | `GuildMemberJoin/Leave/Update` (`OldMember` from the member cache) | already have Server Members; **add Guild Invites** for invite tracking |
| moderation | ban, unban, kick, timeout, with executor and reason | `GuildAuditLogEntryCreate` (pushed over the gateway, no polling) + `GuildBan/Unban` | **add Guild Moderation** |
| server | channel / role / server create, update, delete, incl. the new `CHANNEL_POSITION_UPDATE` (16) and `ROLE_POSITION_UPDATE` (33) audit events | `GuildChannel*`, role events, `GuildUpdate`, audit entries | covered by Guilds + Guild Moderation |
| voice | join, leave, move | `GuildVoiceJoin/Leave/Move` | **add Guild Voice States** |
| automod | rule actions | `AutoModerationActionExecution` | **add AutoMod Execution** |
| guardian | Guardian actions | in-process from `guardian.Service` | none |

None of the new intents is privileged. `privilegedIntents` keeps degrading gracefully, and the logging page shows which categories are affected when Message Content or Server Members is missing.

**Bulk deletes:** disgo v0.20 has no bulk-delete event. Its `MESSAGE_DELETE_BULK` handler (`bot/handlers/message_handler.go:111`) fans out one `MessageDelete` per ID. **Recommendation:** override that one gateway handler with `bot.WithEventManagerConfigOpts(bot.WithGatewayHandlers(...))`, passing disgo's default map with that key replaced. Our handler then sees the whole batch as one event. This is exact; the alternative, a debounce that groups nearby deletes, is a guess. `/purge` (M1) and Guardian's multi-channel delete both go through it.

**Executors:** audit-log entries can arrive before or after the event they explain. A short correlation buffer, keyed by action and target with about a 2-second wait, joins them. With no match, the entry says "executor unknown".

### Overlap with Moderation and Guardian (avoid double posts)

- **Everything goes to the log channel** (PM, 2026-11-26), including Niro's own `/ban`, `/kick`, `/timeout` and Guardian actions, as well as actions taken in Discord's own UI or by other bots. The one exception: when the logging channel for a category *is* the moderation log channel, Niro's own actions are skipped there, because their case card is already in that channel.
- Guardian alerts stay where they are: the alert channel, with Restore, Ban and Keep quarantined buttons. The `guardian` category also posts a short, button-free line, except when its channel is the alert channel.
- Messages deleted by Guardian or `/purge` are logged as "deleted by Niro (Guardian / purge)" rather than "unknown".

### Rate limits and size

- A bounded queue per log channel: one sender per channel, Discord's per-route rate-limit buckets respected, a cap of about 200 entries, then drop with a counter and a single "N entries skipped" line. Raid-mode join floods are grouped into summary entries.
- Valkey message cache: compact JSON (author, channel, text capped at 4,000 characters, attachment names), `SET … EX 7d`, key `niro:logging:msg:<channel>:<id>`. Only guilds with the logging module on and the messages category routed are cached. Expected Valkey size goes into the M5 ops docs.
- New Prometheus metrics: log entries posted, dropped and failed, per category.

## 3. Slices

Each slice is one commit (or a few) on `m3-logging`. CI stays green at every slice.

**Status (2026-11-26):** slices 1–5 are built, plus an ECC Go review whose 11 findings were fixed. The demo checklist is [m3-demo.md](m3-demo.md). Changes from the draft design: moderation, member and server entries come straight from audit-log entries (each carries its executor, so no correlation buffer was needed); deletes of messages that weren't cached are not logged; AutoMod entries show the rule's keyword, never the matched text, when message content is off. Slice 6 waits for the M3 demo.

| # | Slice | Tests | Done check |
|---|---|---|---|
| 1 | **Domain** (`internal/modules/logging`): types, settings + Normalize, ignore rules, edit diff, bulk file renderer, service with fakes, i18n keys + catalog test | Table tests (settings, ignore, diff, truncation); JSON replay fixtures for every event kind through the real service | Every kind produces the expected entry; Niro-executed actions are dropped |
| 2 | **Storage** (`internal/cache/logging.go`): message cache, `Take`, invite snapshot; wiring in `main.go` | Valkey integration tests (TTL expiry, Take, invite diff), plus a contract test against the in-memory cache, as M2 did for the tracker | `go test -race ./...` green with `NIRO_REQUIRE_INTEGRATION=1` |
| 3 | **Discord**: intents, listeners, bulk-delete handler override, audit correlation, send queue, cards, the Guardian line, missing-permission warnings | Adapter unit tests with fake events; queue and correlation timing tests; intent-degradation test | Live smoke test against Niro Dev: commands register, all intents accepted |
| 4 | **API + dashboard**: `openapi.yaml`, regenerate, `internal/api/logging.go`, `LoggingSettings.tsx`, route + nav, en strings | API tests (validation, unknown channel, audit row on save); Vitest for the page | Saving in the dashboard changes routing live, without a restart |
| 5 | **Docs**: `docs/milestones/m3-demo.md` (one row per event kind), changelog, decisions, README status, `graphify update .` | — | PR opened, CI green, **demo with you** |
| 6 | **Release** (only after you approve the M3 demo): see §4 | — | `v0.1.0-alpha.1` tagged, repo public |

## 4. Public-release checklist (`v0.1.0-alpha.1`)

The plan gives the release one line; going public needs more:

1. **Secret scan of the full history** (`gitleaks detect` over all commits). It isn't installed yet. History was rewritten once (decision 9), so scan every ref, then GitHub's own secret scanning once the repo is public.
2. **`SECURITY.md`**: how to report vulnerabilities privately (GitHub private vulnerability reporting, or an address). Essential for a security-first bot.
3. **`CONTRIBUTING.md`**, issue and PR templates. No CLA or DCO (PM, 2026-11-26).
4. **README**: current status, what works, self-host "coming in M5", the source-available wording (not "open source", see decisions notes).
5. **Release artefacts:** source only for the alpha (PM, 2026-11-26). Container images wait for M5 and the self-host guide.
6. **GitHub settings** once public: branch protection on `main` (CI required), Dependabot or Renovate, secret scanning and push protection, private vulnerability reporting.
7. **LICENSE parity check** with NithronOS and NithronAI (byte-identical).
8. Tag `v0.1.0-alpha.1` on `main`, a GitHub release with the changelog, then invite early-tester servers.

## 5. Gaps found while checking the plan

| Gap | Where | Proposal |
|---|---|---|
| Playwright is in the plan (§4, §9) but not set up; the dashboard has only Vitest | `web/` | **Approved:** a small Playwright suite (login stub, toggle a module, save Guardian and Logging settings) in slice 4, run in CI |
| `docs/adr/` was never created; `docs/decisions.md` has played that role | plan §3, §9 | The decisions log is the record; ADRs only for large choices |
| CI has no OpenAPI lint and no `sqlc vet` (plan §4); `generate-check` covers drift only | `.github/workflows/ci.yml` | Add both in slice 4 |
| Plan §6 doesn't list Guild Invites, which invite tracking needs | plan §6 | Record in decisions when slice 3 adds it |
| Plan §2 describes Valkey job streams; the code uses an RPC bridge over a stream plus a reply list | plan §2 | Same idea; note it in decisions |
| `README.md` status still says "M0 Foundations" | README | Fixed in this branch |
| Decisions log had no "M2 accepted" entry, though #6 is merged | decisions | Added as #20 (confirmed) |
