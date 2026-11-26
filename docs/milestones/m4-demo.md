# M4 demo checklist

**Done when** (plan §7): role panels, welcome/goodbye and tickets each work end to end when configured only from the dashboard.

## Setup

1. `make dev-deps`, then `make run`. Migrations 4 (role panels) and 5 (tickets) apply on start.
2. **Re-invite Niro Dev once** with the new invite link (dashboard → server list → Add Niro, or the Installation page). M4 adds **Send Messages in Threads** for ticket threads. Nothing else changes.
3. In the test server, create:
   - two roles below Niro's: `@Red` and `@Blue`;
   - a role with Administrator: `@Admins`;
   - a staff role: `@Support`, given to your main account;
   - channels `#roles`, `#welcome`, `#help` and `#transcripts`;
   - a channel category called **Tickets**.
4. Keep the **second account** from earlier demos: no moderator permissions, not `@Support`.
5. Dashboard → **Modules**: turn on **Role panels**, **Welcome** and **Tickets**.

## Demo

| # | Do | Expect |
|---|---|---|
| 1 | Dashboard → **Role panels** → New panel. Name "Colours", Post in `#roles`, Buttons, Toggle. Add `@Red` (label "Red", emoji 🟥) and `@Blue`. Open the role list | `@Admins` isn't offered (listed as "not offered … dangerous permissions"). The preview shows the message and both buttons as you type |
| 2 | Add a "Text with image" block with an https image, a colour bar, and a link button; **Save and post** | The panel appears in `#roles` looking like the preview |
| 3 | As the second account, press **Red**, then **Red** again | "Added: @Red", then "Removed: @Red" (only visible to them) |
| 4 | Edit the panel: Mode **Pick one**, Save. Press Red, then Blue | Red is added; pressing Blue swaps it ("Added @Blue, Removed @Red"). The posted message updated in place |
| 5 | Edit the panel: Style **Menu**, Save. Pick Red in the menu | Red is toggled; roles you didn't pick are untouched |
| 6 | In Discord, give `@Blue` the Administrator permission, then press Blue on the panel | "Niro can't hand out @Blue any more". Nothing is given. Remove Administrator again |
| 7 | Dashboard → **Welcome**: turn on the welcome message in `#welcome`, add a "{memberCount}" line, tick `@Red` under Roles on join, Save | The preview shows your own name and the server name |
| 8 | Have an account join the server | Welcome posted in `#welcome`, pinging only that member; it gets `@Red`. No DM (off by default) |
| 9 | Turn on the goodbye message in `#welcome`; the account leaves | "… left the server", no ping |
| 10 | Guardian → Verification gate: Verified role `@Blue`, Save. Post the verify panel (`/guardian verify-panel`). Have an account join, then press Verify | Welcomed on join but **no** `@Red` yet; on Verify it gets `@Blue` and `@Red`. Clear the verified role afterwards |
| 11 | Dashboard → **Tickets**: transcripts in `#transcripts`. New panel "Help desk" in `#help` with two categories: **Support** (Private thread, staff `@Support`, one required question "What's wrong?") and **Billing** (Private channel in **Tickets**, staff `@Support`). Save and post | The panel shows two buttons |
| 12 | As the second account, press **Support** and answer the question | A form opens; on submit "Your ticket is open: #ticket-0001", a private thread with the answer, the second account and `@Support` pinged. Others can't see it |
| 13 | Press **Support** again | "You already have an open ticket here: …" (1 per member) |
| 14 | As the second account, press **Claim** | "Only staff can do that." As your main account: "🙋 … claimed this ticket" |
| 15 | `/ticket add` another account (as staff) | They can see and write in the thread |
| 16 | As the second account, press **Close** and give a reason | "🔒 … closed this ticket", the thread is archived and locked, `#transcripts` gets `ticket-0001.html` |
| 17 | Dashboard → Tickets → **Transcript** on ticket 1 | Opens in a new tab: every message, nothing runs (try posting `<script>alert(1)</script>` in a ticket first: it shows as text) |
| 18 | As your main account press **Reopen**, then close again and press **Delete** | Reopened and announced; Delete removes the thread. The transcript stays in the dashboard |
| 19 | Press **Billing** as the second account | A private channel `ticket-0002` in the **Tickets** category, visible only to them, `@Support` and Niro. Close: they lose access, staff keep it with Reopen and Delete |

## Notes

- **Role panels never hand out** managed roles, `@everyone`, roles with dangerous permissions, or roles at or above Niro's. This is checked when saving and on every click.
- **Menus** can't show which roles each member has, so picking in a menu toggles the picked roles and leaves the others alone.
- **Welcome and Guardian:** members removed by the account-age gate aren't greeted. During raid mode, joins are summed up in one message per minute (no DMs or roles). With the verification gate set up, auto-roles come on Verify.
- **Transcripts** are the one place Niro stores message text: gzip HTML in Postgres, deleted after the retention period (90 days by default, 1–365). They open as a standalone page with a `sandbox` Content-Security-Policy, so no script can run.
- **Private-thread tickets** add the opener and up to 25 staff members Niro knows about. Members with Manage Threads see all private threads anyway. Channel tickets use permission overwrites, so every `@Support` member sees them.
