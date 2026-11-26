package discord

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/disgoorg/disgo/discord"

	"github.com/nithron/niro/internal/core/message"
)

func TestRenderDocument(t *testing.T) {
	color := 0x123456
	d := message.Document{AccentColor: &color, Blocks: []message.Block{
		{Type: message.BlockText, Text: "## Hello"},
		{Type: message.BlockSection, Text: "Side", Thumbnail: "https://example.org/t.png"},
		{Type: message.BlockGallery, Images: []message.Image{{URL: "https://example.org/g.png", Description: "alt"}}},
		{Type: message.BlockSeparator, Divider: true, Large: true},
		{Type: message.BlockLinks, Links: []message.Link{{Label: "Site", URL: "https://nithron.com", Emoji: "🌐"}}},
	}}
	if err := d.Validate(message.Budget{Components: 2}); err != nil {
		t.Fatal(err)
	}
	c := renderDocument(d, discord.NewActionRow(discord.NewPrimaryButton("Pick", "/x")))
	if c.AccentColor != color || len(c.Components) != 6 {
		t.Fatalf("container = %+v", c)
	}
	raw, err := json.Marshal(documentMessage(c, nil))
	if err != nil {
		t.Fatal(err)
	}
	s := string(raw)
	for _, want := range []string{`"## Hello"`, `"https://example.org/t.png"`, `"alt"`, `"divider":true`, `"spacing":2`, `"https://nithron.com"`, `"🌐"`, `"/x"`, `"allowed_mentions":{"parse":null`} {
		if !strings.Contains(s, want) {
			t.Errorf("message JSON missing %s:\n%s", want, s)
		}
	}
}
