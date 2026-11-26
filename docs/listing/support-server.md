# Support server setup

The support server is where users get help, hear about releases, and where the team gets alerts. Create it from the Nithron account (it's tied to the creator), then add Niro to it: Niro moderates its own support server.

## Channels

| Category | Channel | Purpose | Who can post |
|---|---|---|---|
| **Start** | `#welcome` | What Niro is, links to the dashboard, docs, Terms, Privacy | staff |
| | `#rules` | Server rules. Includes: "This server uses Niro's server log and tickets; see the Privacy Policy" | staff |
| | `#announcements` | Releases, incidents, changes to the Terms or Privacy Policy | staff (announcement channel) |
| **Help** | `#get-help` | Ticket panel: "Bug", "Question", "Privacy request" | Niro |
| | `#questions` | Forum channel for public questions | everyone |
| | `#status` | Outages and maintenance | staff |
| **Team** (private) | `#ops-alerts` | Alertmanager webhook (see the [runbook](../ops/runbook.md)) | webhook |
| | `#mod-log`, `#server-log` | Niro's moderation log and server log | Niro |
| | `#tickets` | Ticket transcripts | Niro |

## Roles

- **Nithron:** the team. Manage Server, Moderate Members.
- **Support:** volunteers who answer tickets. Ticket staff role.
- **Verified:** given by the verify button below.
- **Announcements:** opt-in ping role, from a role panel.

## Niro configuration (from the dashboard)

- **Moderation:**
  - Mod log channel `#mod-log`.
  - DM the member on actions.
- **Guardian:**
  - Scam links on, with the action set to quarantine.
  - Cross-channel spam on.
  - Raid protection on (10 joins in 30 seconds).
  - Minimum account age of 7 days with the verification gate in `#welcome`, which gives **Verified**.
- **Server log:**
  - Default channel `#server-log`.
  - Ignore `#ops-alerts` and the ticket threads.
- **Welcome:**
  - A short welcome message in `#welcome`, with no DM.
  - Auto-role **Verified** given on verify.
- **Roles:** a role panel in `#welcome` for **Announcements**, in toggle mode.
- **Tickets:**
  - A panel in `#get-help` with three categories, all in thread mode, staff role **Support**.
  - Transcripts to `#tickets`, kept 90 days.
  - The "Privacy request" category has a short form: "What do you need (copy, correction, deletion)?" and "Your Discord user ID".

## Onboarding and safety

- **Community:** turn on Community, and set the Verification Level to Medium.
- **Raids:** enable Discord's own raid protection as a second layer.
- **Invites:** a vanity or permanent invite for the listing and docs, created by an account the team controls.
