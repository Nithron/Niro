# Message builder

Role panels, welcome and goodbye messages, ticket panels and the first message in a ticket all use the same editor. You build the message from blocks and see a live preview next to it.

## Blocks

Press **Add a block** and choose:

| Block | What it shows |
|---|---|
| **Text** | Text with Discord Markdown (bold, italics, lists, links and so on) |
| **Text with image** | Text with a small image beside it (both required) |
| **Images** | A gallery of 1 to 10 images |
| **Spacer** | A gap, with **Show a line** for a divider and **Large gap** for more space |
| **Link buttons** | 1 to 5 buttons that open web pages (http, https or discord links), each with a label, an emoji or both |

Move blocks with **Move up** and **Move down**, or delete them with **Remove block**.

**Colour bar** adds a coloured stripe down the left side of the message.

Feature buttons (role buttons or menus, ticket category buttons) are added by the feature itself, below your blocks.

## Images

Images are added by **Image URL**: an https link to an image that is already online. Discord loads it; Niro never downloads or hosts it. You can't upload files.

Add a **Description (for screen readers)** to each image so members using screen readers know what it shows.

## Variables

Welcome, goodbye and welcome DM messages can use variables such as `{user}` and `{server}`. The list is on the [Welcome](welcome.md#variables) page. Other messages don't replace variables. Unknown variables are left as typed, and the editor flags them.

## Limits

Discord limits how big a message can be. The editor counts as you go ("12 of 40 parts", "350 of 4000 characters"):

| Limit | Value |
|---|---|
| Blocks | 20 |
| Parts (blocks, images, buttons and the feature's own buttons together) | 40 |
| Text, all blocks together | 4,000 characters |
| Images per gallery | 10 |
| Link buttons per row | 5 |
| Link button label | 80 characters |
| URL | 2,048 characters (images: https only) |

If a message is too big, saving fails with a message saying what to remove.

## Preview

The preview imitates how the message looks in Discord. For welcome messages, it uses your own account as the member. Check the result in Discord after posting: small differences are possible.
