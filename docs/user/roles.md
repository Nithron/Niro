# Role panels

A role panel is a message with buttons or a menu that lets members pick their own roles, for example colours, pronouns or notification roles.

## Create a panel

1. Dashboard > your server > **Modules**: turn **Role panels** on.
2. **Role panels** tab > **New panel**.
3. Fill in:
   - **Name**: only shown on the dashboard, to tell panels apart.
   - **Post in**: the channel for the panel.
   - **Style**: **Buttons** or **Menu**.
   - **Mode**: see below.
   - **Roles**: up to 25. Each has a label and an optional emoji. In a menu, each can also have a description.
4. Write the message above the buttons with the [message builder](message-builder.md). The preview updates as you type.
5. Press **Save and post**.

To change a panel, edit it and save. The posted message updates in place. Deleting a panel also deletes its message in Discord. A server can have up to 25 panels.

## Modes

| Mode | What members can do |
|---|---|
| **Toggle** | Add and remove each role. Pressing a button again removes the role |
| **Pick one** | Have only one role from this panel. Picking another swaps it |
| **Verify** | Only add the roles, never remove them. Guardian's minimum account age applies |

With a **Menu**, Discord can't show which roles each member already has. Picking roles in a menu toggles the roles you picked and leaves the others alone.

## Options

- **Most roles per member** (Toggle mode): how many of the panel's roles one member may hold. 0 means no limit.
- **Who can use it**: optionally limit the panel to members with a role. **Everyone** by default.

Members get a reply only they can see, saying exactly what changed, for example "Added: @Red" or "Removed: @Red".

## Roles Niro won't give out

Some roles are never offered and never handed out:

| Role | Why |
|---|---|
| Roles managed by an integration (bot roles, booster role) | Discord doesn't allow it |
| `@everyone` | Everyone has it already |
| Roles with dangerous permissions | Handing them out by button would let anyone take over your server |
| Roles at or above Niro's highest role | Discord doesn't allow it; move Niro's role higher |

Dangerous permissions are: Administrator, Manage Server, Manage Roles, Manage Channels, Manage Webhooks, Ban Members, Kick Members, Timeout Members, Manage Messages, Mention @everyone and Manage Expressions.

The dashboard lists such roles as not offered. Niro checks again on every click, in case a role changed after you saved the panel. If it did, the member is told "Niro can't hand out (role) any more", nothing is given, and the dashboard asks you to remove the role from the panel.

## When it doesn't work

- "Role panels are turned off in this server": turn the module on under **Modules**.
- Nothing changes when members click: check that Niro's role is above the panel's roles in **Server Settings > Roles**.
