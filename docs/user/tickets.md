# Tickets

Tickets are private support conversations between a member and your staff. Members open them from a panel with buttons.

## Before you start

- Create a staff role (for example `@Support`) and give it to your helpers.
- For channel tickets, create a channel category to hold them (optional).
- Dashboard > your server > **Modules**: turn **Tickets** on.

## Create a ticket panel

1. **Tickets** tab > **Panels** > **New panel**.
2. Choose the **Panel** channel, where the panel is posted. Thread tickets are private threads in this channel.
3. Write the panel message with the [message builder](message-builder.md).
4. Add up to 5 categories (one button each). For each category:
   - **Button label** and an optional **Emoji**.
   - **Ticket type**: **Private thread** or **Private channel**.
   - **Create channels in**: the channel category for channel tickets.
   - **Open tickets per member**: 1 by default, up to 5.
   - **Staff roles (they see and handle these tickets)**: 1 to 10 roles.
   - **Form (optional, up to 5 questions)**: each with an optional hint, a **Long answer** option and a **Required** option.
   - **First message in the ticket**: what Niro posts when the ticket opens.
5. Press **Save and post**. A server can have up to 10 ticket panels.

## Thread or channel

| | Private thread | Private channel |
|---|---|---|
| Where | In the panel's channel | In the category you chose |
| Who sees it | The opener, plus up to 25 staff members Niro knows about. Members with Manage Threads see all private threads anyway | The opener, everyone with a staff role, and Niro |
| Best for | Small teams | Larger teams: most reliable |

For thread tickets, make the staff role mentionable so everyone with it is added.

## Opening a ticket

A member presses a category button. If the category has a form, it opens first. Niro then creates the ticket, posts the answers and pings the member and staff. The member gets a reply like "Your ticket is open: #ticket-0001". Ticket numbers are counted per server.

If they already have the most open tickets the category allows, Niro links to their open ticket instead.

## Handling a ticket

Buttons in the ticket:

| Button | Who can use it | What it does |
|---|---|---|
| **Claim** | Staff | Shows that you are handling it |
| **Close** | The opener or staff | Asks for an optional reason, saves the transcript and hides the ticket from the opener |
| **Reopen** | Staff | Reopens a closed ticket |
| **Delete** | Staff | Deletes the closed thread or channel. The transcript stays in the dashboard |

When a thread ticket closes, the thread is archived and locked. When a channel ticket closes, the opener loses access and staff keep it.

"Staff" means members with one of the category's staff roles, plus the server owner, Administrators and members with Manage Server.

## Commands

Use these inside a ticket:

| Command | Who | What it does |
|---|---|---|
| `/ticket add user` | Staff | Add someone to the ticket |
| `/ticket remove user` | Staff | Remove someone (not the opener, staff or Niro) |
| `/ticket close [reason]` | The opener or staff | Close the ticket |
| `/ticket claim` | Staff | Claim the ticket |

## Transcripts

When a ticket closes, Niro saves a transcript of its messages as a web page.

- **Also post transcripts in**: optionally, Niro posts the transcript as a file (for example `ticket-0001.html`) in a channel you choose.
- On the **Tickets** tab, the ticket list shows each ticket's **Status**, **Category** and **Opened by**. Press **Transcript** to open it. Nothing in a transcript can run: it is shown as plain content.
- **Keep transcripts for (days)**: 1 to 365 days, 90 by default. After that the transcript is deleted.

Transcripts are the only place Niro stores message text. Tell your members, for example in your server rules. A ticket can only be deleted once its transcript is saved; if saving failed, reopen and close it again.

## Permissions Niro needs

- Thread tickets: Create Private Threads, Send Messages in Threads and Manage Threads in the panel's channel.
- Channel tickets: Manage Channels in the category you chose.
- In every ticket: View Channels, Send Messages, Embed Links, Attach Files and Read Message History (for transcripts).
- In the transcript channel: Send Messages and Attach Files.

All of these are in the permissions Niro asks for when you add it. Niro checks before opening a ticket and says why if it can't.
