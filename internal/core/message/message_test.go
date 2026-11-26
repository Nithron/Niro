package message

import (
	"errors"
	"strings"
	"testing"
)

func TestValidateAcceptsAGoodDocument(t *testing.T) {
	color := 0x4CB8FF
	d := Document{AccentColor: &color, Blocks: []Block{
		{Type: BlockText, Text: "  ## Welcome to {server}!  "},
		{Type: BlockSection, Text: "Read the rules", Thumbnail: "https://example.org/a.png", Images: []Image{{URL: "x"}}},
		{Type: BlockSeparator, Divider: true, Text: "ignored"},
		{Type: BlockGallery, Images: []Image{{URL: "https://example.org/b.png", Description: "banner"}}},
		{Type: BlockLinks, Links: []Link{{Label: "Website", URL: "https://nithron.com", Emoji: "🌐"}, {Label: "App", URL: "discord://-/channels/1"}}},
	}}
	if err := d.Validate(Budget{}); err != nil {
		t.Fatal(err)
	}
	if d.Version != Version || d.Blocks[0].Text != "## Welcome to {server}!" {
		t.Errorf("not normalized: %+v", d.Blocks[0])
	}
	if d.Blocks[1].Images != nil || d.Blocks[2].Text != "" {
		t.Error("fields of other block types were kept")
	}
	// container 1 + text 1 + section 3 + separator 1 + gallery 1 + row 1 + 2 buttons
	if n := d.Components(); n != 10 {
		t.Errorf("components = %d", n)
	}
}

func TestValidateRejects(t *testing.T) {
	bad := -1
	long := strings.Repeat("a", MaxTextTotal+1)
	cases := map[string]Document{
		"empty":            {},
		"only separators":  {Blocks: []Block{{Type: BlockSeparator}}},
		"unknown block":    {Blocks: []Block{{Type: "video"}}},
		"version":          {Version: 9, Blocks: []Block{{Type: BlockText, Text: "x"}}},
		"colour":           {AccentColor: &bad, Blocks: []Block{{Type: BlockText, Text: "x"}}},
		"empty text":       {Blocks: []Block{{Type: BlockText, Text: "   "}}},
		"http image":       {Blocks: []Block{{Type: BlockSection, Text: "x", Thumbnail: "http://example.org/a.png"}}},
		"javascript link":  {Blocks: []Block{{Type: BlockLinks, Links: []Link{{Label: "x", URL: "javascript:alert(1)"}}}}},
		"credentials link": {Blocks: []Block{{Type: BlockLinks, Links: []Link{{Label: "x", URL: "https://a:b@example.org"}}}}},
		"too many links":   {Blocks: []Block{{Type: BlockLinks, Links: make([]Link, 6)}}},
		"empty gallery":    {Blocks: []Block{{Type: BlockGallery}}},
		"text emoji":       {Blocks: []Block{{Type: BlockLinks, Links: []Link{{Label: "x", URL: "https://a.org", Emoji: "<:x:1>"}}}}},
		"too much text":    {Blocks: []Block{{Type: BlockText, Text: long}}},
		"too many blocks":  {Blocks: make([]Block, MaxBlocks+1)},
	}
	for name, d := range cases {
		if err := d.Validate(Budget{}); !errors.Is(err, ErrInvalid) {
			t.Errorf("%s: err = %v", name, err)
		}
	}
}

func TestValidateLeavesRoomForTheFeature(t *testing.T) {
	d := Simple("Pick your roles")
	if err := d.Validate(Budget{Components: 30}); err != nil {
		t.Fatalf("panel with 25 buttons: %v", err)
	}
	var blocks []Block
	for range 9 {
		blocks = append(blocks, Block{Type: BlockSection, Text: "x", Thumbnail: "https://a.org/x.png"})
	}
	d = Document{Blocks: blocks} // 1 + 27 = 28 components
	if err := d.Validate(Budget{Components: 30}); !errors.Is(err, ErrInvalid) {
		t.Errorf("over the component limit: %v", err)
	}
	// A feature with buttons may send a document without blocks.
	empty := Document{}
	if err := empty.Validate(Budget{Components: 2}); err != nil {
		t.Errorf("empty document with feature controls: %v", err)
	}
}

func TestRenderVariables(t *testing.T) {
	d := Document{Blocks: []Block{{Type: BlockText, Text: "Hi {user} ({user.name}, {user.id}) — welcome to {server}, member #{memberCount}! See {channel}. {unknown} {User}"}}}
	esc := func(s string) string { return strings.ReplaceAll(s, "*", "\\*") }
	out := d.Render(Vars{UserMention: "<@1>", UserName: "a*b", UserID: "1", Server: "S*", MemberCount: 42, Channel: "<#9>"}, esc)
	want := "Hi <@1> (a\\*b, 1) — welcome to S\\*, member #42! See <#9>. {unknown} {User}"
	if out.Blocks[0].Text != want {
		t.Errorf("rendered %q", out.Blocks[0].Text)
	}
	if d.Blocks[0].Text == want {
		t.Error("Render changed the original document")
	}
	if got := d.UnknownVariables(); len(got) != 2 || got[0] != "unknown" || got[1] != "User" {
		t.Errorf("unknown variables = %v", got)
	}
}

func TestIsEmoji(t *testing.T) {
	for s, want := range map[string]bool{"🎨": true, "👩‍💻": true, "❤️": true, "a": false, "": false, ":x:": false, "<:a:1>": false, "🎨 art": false} {
		if IsEmoji(s) != want {
			t.Errorf("IsEmoji(%q) = %v", s, !want)
		}
	}
}
