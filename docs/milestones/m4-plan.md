# M4 implementation plan: Community toolkit

*Draft, 2026-11-26. Built from plan §7 (M4), the M0–M3 code and a check of disgo v0.20. Items marked **PM** need your decision before the slice named.*

**Done when** (plan §7): role panels, welcome/goodbye and tickets each work end to end when configured only from the dashboard.

## 1. What already exists

- `core.ModuleRoles`, `ModuleWelcome` and `ModuleTickets` are registered (off by default), so toggles, settings, live reload and config audit already work. Each gets a `SettingsSpec` as Guardian and Logging did.
- Reusable pieces:
  - The RPC bridge (`internal/rpc`). The API asks the bot to post or update panel messages, so the bot token stays in the bot process.
  - `guild.meta` over RPC, for channel and role pickers.
  - Components V2 helpers and `noPings`.
  - `core.CanActOn` and rank helpers, for role hierarchy checks.
  - Guardian's verification gate (`guardian.CanVerify`, the `VerifyRoleID` setting).
  - Logging's transcript writer, a pattern for the ticket transcript.
- Patterns to copy:
  - Domain package plus fakes and replay tests (`internal/modules/guardian`, `logging`).
  - Store and migrations (`internal/store`, migration 4).
  - Settings API with reference checks (`internal/api/logging.go`).
  - Dashboard settings pages, Vitest and the Playwright suite.
- No new gateway intent. Server Members (joins, leaves) and Guild Messages already cover everything.

## 2. Design

### 2.1 Shared message builder (`internal/core/message` + `web/src/components/builder`)

- **One document format** (JSON, versioned) for every message Niro posts on a server's behalf: role panels, welcome and goodbye, ticket panels and the ticket opening message.
- **Blocks:** an accent colour, text (Markdown), a section with a thumbnail, a media gallery (image URLs), a separator, and link buttons. Feature-owned controls go in a reserved slot: role buttons and selects, and the "Open ticket" button.
- **Validation in Go:** Discord's Components V2 limits (40 components, 4,000 characters of text, https image URLs only), shared by API and bot. Niro never fetches the images; Discord does.
- **Variables** (`{user}`, `{user.name}`, `{server}`, `{memberCount}`, `{channel}`) are rendered by one function and escaped. Unknown variables are left as typed and flagged in the editor.
- **Dashboard:** a block editor with a live preview that imitates Discord's Components V2 look (dark and light). Each feature page embeds it.

### 2.2 Role panels (`internal/modules/roles`, table `role_panels`)

- A panel is a message document plus its options:
  - Style: buttons, or a select menu.
  - Mode: **toggle** (add or remove), **unique** (one role from the panel at a time) or **verify** (add only, never removable from the panel).
  - Up to 25 roles, each with a label, emoji and description.
  - Optional limits: a maximum number of roles from the panel, and a required role.
- **Posting:** the dashboard saves, then the bot posts or edits the message over RPC. The table keeps the channel and message IDs, so editing a panel updates the posted message.
- **Security on every click.** The custom ID carries only the panel and role ID. The bot re-reads the panel from the store and checks:
  - the role is still in the panel;
  - the role isn't managed or `@everyone`;
  - Niro's top role is above it.
  
  Roles with dangerous permissions (Administrator, Manage Server, Manage Roles, Ban, Kick and so on, reusing Guardian's `DangerousPermissions` list) **cannot be put on a panel at all**: the API rejects them, and the bot rechecks on every click in case the role's permissions changed later.
- **Verify mode** shares Guardian's minimum-account-age check, so the two gates can't disagree.
- Replies are ephemeral and say exactly what changed ("Added @Artist, removed @Musician").

### 2.3 Welcome / goodbye (`internal/modules/welcome`, settings only)

- **Welcome:** a channel message and an optional DM, each a builder document with variables.
- **Goodbye:** a channel message.
- **Auto-roles on join:** up to 10 roles, with the same dangerous-permission ban as panels. **Respecting the verification gate:** when Guardian's gate is set up, auto-roles are given when the member verifies, not on join. Members cut by Guardian's account-age gate are never greeted.
- **During raid mode** welcome messages are batched into one line ("12 members joined during raid mode") instead of 12 posts.
- **Preview:** the dashboard renders the preview with your own account as `{user}`.

### 2.4 Tickets (`internal/modules/tickets`, tables `ticket_panels`, `tickets`, `ticket_transcripts`)

- **Panels:** a message document with one button per category (up to 5).
- **Each category has:**
  - a mode: **private thread** in the panel's channel, or a **channel** in a chosen category;
  - staff roles;
  - an optional form of up to 5 questions (a modal with text inputs);
  - a per-user limit of open tickets (default 1).
- **Lifecycle:** open → claimed → closed → reopened (within the retention window) or deleted.
  - Buttons in the ticket: **Claim**, **Close** (with an optional reason), **Reopen**.
  - Slash commands as fallbacks: `/ticket add|remove|close|claim`.
  - Ticket numbers are per server, like cases.
- **Who can see a ticket:**
  - In thread mode, the opener and staff are added to a private thread.
  - In channel mode, the channel's overwrites allow the opener and staff only.
  - Niro checks it can do this before opening, and says why if it can't.
- **Transcripts:** on close, Niro reads the ticket's messages and renders a self-contained HTML file. It contains no scripts, and every user value is escaped. The file is gzip-compressed and stored with an expiry (approved default: 90 days, configurable 1–365). It is also posted as a file to a transcript channel, if one is set. The dashboard lists tickets and opens transcripts in a sandboxed viewer (`Content-Security-Policy: sandbox`). A daily job deletes expired transcripts.
- **This is the one place Niro stores message text in Postgres.** It was already approved in plan §5 (90-day ticket transcripts). It needs saying clearly in the docs and privacy policy (decision to record).

### Permissions

**Send Messages in Threads** is **not** in the invite set (`permissions.go`), and thread-mode tickets need it. Proposal: add it to the invite. Servers that already invited Niro would grant it with one click on the re-invite link; the dashboard can show a "missing permission" notice. Least privilege still holds: no Administrator. **PM**

## 3. Slices

| # | Slice | Tests | Done check |
|---|---|---|---|
| 1 | **Message builder:** Go document model, validation, variables, Components V2 rendering; dashboard editor + live preview | Table tests for limits and escaping; render golden tests; Vitest for the editor | A document round-trips dashboard → API → Discord message |
| 2 | **Role panels:** migration 4, store, domain, RPC post/edit, click handling, API, dashboard page | Domain tests (modes, limits, dangerous roles), store integration tests, adapter tests, API tests, Vitest, Playwright | A panel built in the dashboard posts, and clicks add/remove roles correctly in the test server |
| 3 | **Welcome / goodbye:** settings, join/leave handling, auto-roles with the verification gate, raid batching, API, dashboard | Domain tests incl. gate and raid cases; API tests; Vitest | Joining/leaving the test server posts the configured messages and roles |
| 4 | **Tickets:** migration, store, domain, panels, modal form, thread/channel creation, claim/close/reopen, transcripts, retention job, API, dashboard list + transcript viewer | Domain lifecycle tests, store integration, HTML-escaping tests for transcripts, adapter tests, API tests, Vitest, Playwright | A ticket opened from a panel can be claimed, closed (transcript stored and posted) and reopened |
| 5 | **Docs:** `m4-demo.md`, changelog, decisions, README, privacy note on transcripts, `graphify update .` | — | PR, CI green, **demo with you** |

Each slice is reviewed by an ECC reviewer (Go, plus React for the builder) before it is merged into the branch, as in M3.

## 4. Questions for the PM

1. **Send Messages in Threads:** add it to the invite permissions for thread-mode tickets (recommended)? Otherwise tickets support channel mode only.
2. **Transcripts:** stored in Postgres for 90 days and viewable in the dashboard, as plan §5 says (recommended)? Or only posted as a file to a channel, with nothing stored?
3. **Images in the message builder:** image URLs only for now (recommended)? Discord loads them; Niro hosts nothing. Uploads would need file storage, which plan §8 puts with premium.
4. **Limits:** 25 role panels, 10 ticket panels, 5 categories per panel and 1 open ticket per user by default. OK?
5. **Welcome DMs:** off by default (recommended, since unsolicited DMs annoy people)?
