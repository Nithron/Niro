# Niro and the Nithron identity: proposal

**Settled on 2026-11-27, evening** (Thiago, through the website session; DESIGN-APPS.md in nithron-site has the full note). This note wins over the proposal below where they disagree:

- The interface has no hue. Wherever this proposal says `--main`, read ink.
- No drawings for now: sign-in, empty screens and setup are words and one action. Icons become one shared pixel set on a 16 by 16 grid, drawn by the website session for all three products; Niro routes every icon through `web/src/components/Icon.tsx` and draws none of its own. The emojis and the avatar become pixel art too, later.
- The tokens package is under the NCL v1.0, so Niro carries `line.css` (vendored whole).
- Decisions 3 and 5 below are answered: the preview stays in Discord's look, framed and labelled. Niro is no longer last: all three products lay their foundations now; the visible screens follow in the order nOS, nAI, Niro. The family bar and the marks are switched on by a setting of the hosted deployment, off by default.
- Phase 1 (steps 1 and 2) is done on branch `ui/identity`. Phase 2 (step 3) starts after the website session's review.
- **2026-11-28:** Thiago picked the mark: D, "In view" (decision 34). The kit is in `assets/brand`, on one sheet in `docs/design/brand/kit.html`.

For Thiago's approval. Nothing changes in Niro until this proposal is approved. Niro is last in the order (nOS, then nAI). It follows the chapter for applications (nithron-site, DESIGN-APPS.md) and the line direction (DESIGN-V3.md). Written 2026-11-27.

## Where Niro stands today

- **Stack:** React 19, Vite, Tailwind 4. All colours are twelve variables in `web/src/styles.css`, dark by default and light by the system. Tailwind reads them with `@theme inline`.
- **Components:** shared parts are in `web/src/components/ui.tsx`: Button, LinkButton, Card, Switch, SwitchRow, Field, Select, TextInput, TextArea, Badge, Spinner, ErrorState, GuildIcon and Logo. The screens use those parts and Tailwind classes.
- **Departures from the chapter:**
  - a brand gradient (`--brand` to `--brand-2`) on the first button and the logo;
  - tinted cards (`bg-surface`, 10 places);
  - rounded panels (`rounded-lg`/`rounded-xl`, 23 places);
  - 2 shadows;
  - a spinner;
  - no mono face (5 places use the system's);
  - Mona Sans is named but not shipped, so every visitor sees a fallback face.
- **Outside the dashboard, Niro has a second face, inside Discord:**
  - Components V2 messages with a colour bar (brand, success, warning, danger, and one colour per log category);
  - seven app emojis (niro, shield, ok, warn, error, info, lock);
  - the bot's avatar;
  - ticket transcripts (a standalone HTML page in Discord's look).

## Screen by screen

| Screen | Today | With the identity |
|---|---|---|
| **Layout** (all signed-in screens) | Top bar with the logo, server switcher, account menu; side navigation per server; footer | The family bar on top (niro.nithron.com only, see "Self-hosted copies"). Navigation in words, the current item in ink with a 2 px `--main` mark. One ground, panels separated by hairlines. The account menu is a popover with an edge line, no shadow |
| **Sign-in** | Card with the logo and a gradient button | The page's one drawing: a line figure of the thing itself (a shield over a message line, or a character drawing of a moderation log). One sentence, one ink button "Log in with Discord", Terms and Privacy under it |
| **Servers** | Grid of cards with server icons; "Add Niro" buttons | A ruled list, one row per server: icon, name, a word for the state ("Niro is in this server" / "Not added"), the action at the end as a line button. Server icons stay (they are data) |
| **Server overview** (modules) | Cards with switches; the not-present card; Delete data | A ruled table of modules: name, one line of what it does, the switch. The not-present state is a full-width panel with one action. **Delete data…** is a line button that says what is destroyed, confirmed in a dialog, no red fill |
| **Moderation settings, Guardian settings, Logging, Welcome** | Forms in cards | One column per panel, labels above fields, 44 px fields with an edge line, help in fine text, one ink "Save" per form. Channel and role pickers keep their search. Role colours from Discord show as a small mark beside the role's name (data, not decoration) |
| **Cases** (list and detail) | Table in a card, filters, paging | Ruled table without a card: case number, user ID and times in the mono face, numbers right-aligned. The action (warn, timeout, kick, ban) is a word, not a colour. Detail is a two-column definition list with the reason editable in place |
| **AutoMod** | Preset cards, rule editor | Presets as a ruled list with "Add" line buttons; the editor as one form. Rule state (on, off) is a mark and a word |
| **Guardian detections** | Table with Restore, Ban, Keep buttons; raid switch | Ruled table. State uses the status roles: open quarantine = "earlier" (waiting), restored = pass, banned = a word with no colour. Review buttons are line buttons; the raid-mode switch is the one control set apart, with its state as a mark and a word ("Raid mode on" in `--status-earlier`) |
| **Security audit** | List of findings with severity badges | Findings as a ruled list: a cross for "fix this", an open square for "check this", a filled square for "in order", each with its word. No coloured rows |
| **Role panels, Welcome, Tickets** (with the message builder) | Editor plus a Discord-like preview | Editor in the identity; the **preview stays in Discord's look** (it shows exactly what members will see), framed by a hairline and labelled "Preview in Discord" |
| **Ticket transcripts** | Standalone HTML, Discord's look | Restyled in the line identity, inline (no external files), both themes, the mono face for times and IDs. It is a Niro document, not a picture of Discord |
| **Terms, Privacy** | Markdown pages | Text at the site's measure, headings from the type scale, the family bar on top |
| **Empty and loading** | Spinner, short text | Loading shows the ruled layout with mono words and the blinking cursor. Empty screens with nothing yet (no cases, no detections, no panels) get one sentence and one action. A drawing only where a whole screen is empty, not in tables |
| **Errors** | ErrorState with a retry | At the place of the failure: a cross, the words, what to do. No toast alone (Niro has no toasts today; keep it that way) |

## Inside Discord

The chapter covers the dashboard. Discord shows Niro's messages on its own grey grounds, in its own dark and light themes, with no way to follow Nithron's. Proposal:

- **Colour bars** follow the roles, never a hue name in code:
  - `main` for neutral replies and panels;
  - `pass` for "done";
  - `earlier` for warnings, lockdowns and pending quarantines;
  - `failed` for errors.

  Log entries drop the colour per category and take `main`, or no bar; a moderation action gets no colour of its own. Discord takes one fixed colour per bar for both its themes, so each role takes one fixed value that reads on both. The design guide proposes the light theme's bar value; nithron-site adds that token once the main colour is chosen.
- **Emojis:** the seven app emojis redrawn as fine-line figures in the identity's style (the `<N>` mark, a shield, a filled square, an open square, a cross, an "i", a lock). Discord's dark and light grounds are greys, not black or white, so the stroke is a mid grey that reads on both, with colour only on the state marks.
- **Avatar:** the `<N>` mark as a line drawing, ink on the ground. Discord crops it to a circle.

## Self-hosted copies

Anyone may run their own Niro (NCL). Their copies must not carry Nithron's family bar or Niro's name and logo (TRADEMARK_POLICY.md). So the family bar and the brand marks are switched on by the hosted deployment (a setting), not built into every copy. A self-hosted copy gets the same identity, minus the bar and the marks.

## Order of work, once approved

1. **Tokens:** install the packed `@nithron/tokens` and map the twelve variables onto it in `styles.css` (the chapter's table). Remove `--brand-2` and the gradient. The same file names the three states (`--success`, `--warning` and `--danger` become ok, warning and failed). Notices that aren't a state, like the not-present card or an info line, lose their tint. Ship Mona Sans and Monaspace Neon from the dashboard. Add a lint rule against hex values and raw palette classes. Most screens follow from this step alone.
2. **ui.tsx:** square panels, ink first action, line buttons, the switch, the badge as a mark and a word, the loading state, and the focus ring in `--main`. A confirm dialog of Niro's own replaces the six browser confirms (deleting a case, a role panel, a ticket panel or an AutoMod rule; a ban from Detections; starting raid mode), as the guide asks for destructive actions. A test fails the build on hex values and raw palette classes outside the Discord preview.
3. **Screens,** in the order of use: Layout and the family bar, Servers, Server overview, Cases, Detections, the settings forms, then the rest.
4. **Discord side:** colour roles in the bot, new emojis and avatar (one release, after the dashboard).
5. **Transcripts.**

Every step is checked at AA in both themes. Playwright and Vitest keep running. No deployment to niro.nithron.com without your go-ahead.

## Your decisions

1. The main colour (open for all three products).
2. The tokens package's licence: Niro's repository is public. It can carry the tokens under a licence that allows that (MIT, as the packed archive suggests), not under UNLICENSED.
3. Whether the message preview stays in Discord's look (proposed: yes).
4. Whether the emojis and avatar change in the same release as the dashboard (proposed: just after).
5. Niro last, or as a small pilot.

## Questions held for Thiago

Collected while he was away on 2026-11-27. The safe default was taken for each and is noted; nothing below is pushed, merged or deployed.

1. **Answered 2026-11-27:** #10 merged into `main` and #11 into `m5-hardening`; `m5-hardening` still has to reach `main` with his word. **Push and merge `ui/identity`.** It is local only, branched from `m5-hardening` (PR #10, not merged). Default: it stays local until you have seen it. Once M5 is approved, do you want it as its own PR after #10?
2. **The ESLint colour rule.** A config-protection hook in the coding session blocks edits to `web/eslint.config.js`. The Vitest test in `src/test/identity.test.ts` does the same check in CI. Default: the test only. Allow the edit if you want the lint rule too.
3. **Monaspace Neon at 511 KB.** It is the complete upstream variable font, kept whole because of the reserved name. It loads on first use and is then cached for good. Default: keep it whole until the pixel display face takes over some of its uses.
4. **A self-hosted copy's name.** With `NIRO_NITHRON_SITE` off, the dashboard shows no family bar and no logo, but still says "Niro" in type and in its texts. TRADEMARK_POLICY.md asks self-hosters to use their own brand. Default: unchanged. Should a self-hoster be able to set their own name (a setting), or is that their fork's job?
5. **Member counts on the overview.** The website session asked for server and member counts as large numbers. The API has no member count. Adding one is an API and bot change. Default: the large numbers are what the API already gives (servers with Niro, modules on, cases). Should a member count be added to the guild API?
6. **"We" in the Terms and the Privacy Policy.** Both define "we" as Nithron, the sole trader, and are live, versioned legal texts (1.0, 2026-11-26) that promise notice before a change. Every other document now speaks as "Niro", "Nithron" or "I". Default: the legal texts are unchanged. Reword them in a new version?
7. **`.impeccable/config.json`.** A design-check plugin wrote it when its "overused font" finding on Mona Sans was suppressed, since Mona Sans is the face the tokens name. Default: not committed.
