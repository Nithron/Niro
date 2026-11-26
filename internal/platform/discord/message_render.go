package discord

import (
	"github.com/disgoorg/disgo/discord"

	"github.com/nithron/niro/internal/core/message"
)

// renderDocument turns a message document into one Components V2 container.
// extra are the feature's own controls (role buttons, the ticket button…),
// added after the document's blocks. The document must have been validated.
func renderDocument(d message.Document, extra ...discord.ContainerSubComponent) discord.ContainerComponent {
	comps := make([]discord.ContainerSubComponent, 0, len(d.Blocks)+len(extra))
	for _, b := range d.Blocks {
		switch b.Type {
		case message.BlockText:
			comps = append(comps, discord.NewTextDisplay(b.Text))
		case message.BlockSection:
			comps = append(comps, discord.NewSection(discord.NewTextDisplay(b.Text)).
				WithAccessory(discord.NewThumbnail(b.Thumbnail)))
		case message.BlockGallery:
			items := make([]discord.MediaGalleryItem, len(b.Images))
			for i, img := range b.Images {
				items[i] = discord.MediaGalleryItem{Media: discord.UnfurledMediaItem{URL: img.URL}, Description: img.Description}
			}
			comps = append(comps, discord.NewMediaGallery(items...))
		case message.BlockSeparator:
			sep := discord.NewSmallSeparator()
			if b.Large {
				sep = discord.NewLargeSeparator()
			}
			comps = append(comps, sep.WithDivider(b.Divider))
		case message.BlockLinks:
			buttons := make([]discord.InteractiveComponent, len(b.Links))
			for i, l := range b.Links {
				btn := discord.NewLinkButton(l.Label, l.URL)
				if l.Emoji != "" {
					btn = btn.WithEmoji(discord.ComponentEmoji{Name: l.Emoji})
				}
				buttons[i] = btn
			}
			comps = append(comps, discord.NewActionRow(buttons...))
		}
	}
	comps = append(comps, extra...)
	c := discord.NewContainer(comps...)
	if d.AccentColor != nil {
		c = c.WithAccentColor(*d.AccentColor)
	}
	return c
}

// documentMessage is a Components V2 message with the given container.
func documentMessage(c discord.ContainerComponent, mentions *discord.AllowedMentions) discord.MessageCreate {
	if mentions == nil {
		mentions = noPings
	}
	return discord.MessageCreate{
		Flags: discord.MessageFlagIsComponentsV2, Components: []discord.LayoutComponent{c}, AllowedMentions: mentions,
	}
}
