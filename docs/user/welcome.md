# Welcome and goodbye

The Welcome module greets new members, says goodbye when they leave, and can give roles on join.

## Set it up

1. Dashboard > your server > **Modules**: turn **Welcome** on.
2. **Welcome** tab:
   - **Welcome message**: tick **Post a welcome message**, choose a **Channel**, and write the message.
   - **Notify the new member**: the message pings only the member who joined, never `@everyone` or roles.
   - **Direct message**: tick **Send new members a DM** and write the DM.
   - **Goodbye message**: tick **Post a goodbye message**, choose a **Channel**, and write it. Goodbyes don't ping anyone.
   - **Roles on join**: pick up to 10 roles.
3. Save. Changes apply immediately.

The messages use the [message builder](message-builder.md). The preview shows your own name and your server's name.

## Variables

Type these in the message text. Niro replaces them when it posts.

| Variable | Becomes |
|---|---|
| `{user}` | A mention of the member |
| `{user.name}` | The member's name |
| `{user.id}` | The member's user ID |
| `{server}` | Your server's name |
| `{memberCount}` | How many members the server has |
| `{channel}` | A mention of the welcome channel (in a goodbye, the goodbye channel) |

Unknown variables are left as typed, and the editor points them out.

## Direct messages

Welcome DMs are **off by default**: many people find unsolicited DMs annoying. Members who have DMs closed simply get nothing.

## Roles on join

- Up to 10 roles.
- Roles managed by integrations or with dangerous permissions are never given (the same rules as [role panels](roles.md#roles-niro-wont-give-out)).
- Niro's role must be above these roles.

## How Welcome works with Guardian

Guardian comes first:

- **Verification gate**: when [Guardian's verification gate](guardian.md#verification-gate) is set up, members are welcomed when they join, but roles on join are given only when they press **Verify**.
- **Account-age gate**: members removed by the account-age gate aren't greeted.
- **Raid mode**: joins during raid mode are summed up in one message per minute, for example "12 members joined during raid mode. Niro didn't greet them one by one." They get no DMs and no roles.
