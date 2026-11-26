// Package message is Niro's message document: the format of every message a
// server designs in the dashboard (role panels, welcome and goodbye, ticket
// panels). It is platform-agnostic; the Discord adapter renders a Document
// as a Components V2 container, and the dashboard previews the same format.
package message

import (
	"errors"
	"fmt"
	"net/url"
	"regexp"
	"slices"
	"strings"
	"unicode/utf8"
)

// Version is the current document version.
const Version = 1

// BlockType is one kind of block.
type BlockType string

const (
	BlockText      BlockType = "text"      // Markdown text
	BlockSection   BlockType = "section"   // text with a thumbnail image beside it
	BlockGallery   BlockType = "gallery"   // up to 10 images
	BlockSeparator BlockType = "separator" // a gap, optionally with a line
	BlockLinks     BlockType = "links"     // a row of up to 5 link buttons
)

// Document is a designed message.
type Document struct {
	Version int `json:"version"`
	// AccentColor is the container's colour bar (0xRRGGBB); nil for none.
	AccentColor *int    `json:"accent_color,omitempty"`
	Blocks      []Block `json:"blocks"`
}

// Block is one part of a document. Which fields apply depends on Type.
type Block struct {
	Type BlockType `json:"type"`
	// Text: text and section blocks.
	Text string `json:"text,omitempty"`
	// Thumbnail: section blocks (an https image URL).
	Thumbnail string `json:"thumbnail,omitempty"`
	// Images: gallery blocks.
	Images []Image `json:"images,omitempty"`
	// Divider and Large: separator blocks.
	Divider bool `json:"divider,omitempty"`
	Large   bool `json:"large,omitempty"`
	// Links: link-button blocks.
	Links []Link `json:"links,omitempty"`
}

// Image is an image shown by URL; Discord loads it, Niro never fetches it.
type Image struct {
	URL         string `json:"url"`
	Description string `json:"description,omitempty"` // alt text
}

// Link is a link button.
type Link struct {
	Label string `json:"label"`
	URL   string `json:"url"`
	Emoji string `json:"emoji,omitempty"` // a Unicode emoji
}

// Limits (Discord's Components V2 limits, with room for feature controls).
const (
	MaxComponents   = 40   // Discord: per message, counting nested ones
	MaxTextTotal    = 4000 // Discord: all text displays together
	MaxBlocks       = 20
	MaxGalleryItems = 10
	MaxLinks        = 5
	MaxLinkLabel    = 80
	MaxURL          = 2048
	MaxAltText      = 1024
)

// ErrInvalid wraps every validation failure.
var ErrInvalid = errors.New("invalid message")

func invalid(format string, a ...any) error {
	return fmt.Errorf("%w: %s", ErrInvalid, fmt.Sprintf(format, a...))
}

// Budget is what a feature adds to a document (its buttons or menus), so the
// whole message stays within Discord's limits.
type Budget struct {
	Components int // components the feature adds
	Text       int // characters of text the feature adds
}

// Components counts the document's Discord components: the container, plus
// each block's own components.
func (d Document) Components() int {
	n := 1 // the container
	for _, b := range d.Blocks {
		switch b.Type {
		case BlockText, BlockSeparator:
			n++
		case BlockSection:
			n += 3 // section, its text, its thumbnail
		case BlockGallery:
			n++ // gallery items are not components
		case BlockLinks:
			n += 1 + len(b.Links) // action row + buttons
		}
	}
	return n
}

// TextLength is the number of characters of text in the document.
func (d Document) TextLength() int {
	n := 0
	for _, b := range d.Blocks {
		n += utf8.RuneCountInString(b.Text)
	}
	return n
}

// Validate checks the document against Discord's limits, leaving room for
// what the feature adds. It also normalizes it (sets the version, trims).
func (d *Document) Validate(reserve Budget) error {
	if d.Version == 0 {
		d.Version = Version
	}
	if d.Version != Version {
		return invalid("unsupported version %d", d.Version)
	}
	if d.AccentColor != nil && (*d.AccentColor < 0 || *d.AccentColor > 0xFFFFFF) {
		return invalid("accent colour must be between #000000 and #FFFFFF")
	}
	if d.Blocks == nil {
		d.Blocks = []Block{}
	}
	if len(d.Blocks) > MaxBlocks {
		return invalid("at most %d blocks", MaxBlocks)
	}
	hasContent := false
	for i := range d.Blocks {
		b := &d.Blocks[i]
		b.Text = strings.TrimSpace(b.Text)
		if err := b.validate(i + 1); err != nil {
			return err
		}
		if b.Type != BlockSeparator {
			hasContent = true
		}
	}
	if !hasContent && reserve.Components == 0 {
		return invalid("the message is empty")
	}
	if n := d.Components() + reserve.Components; n > MaxComponents {
		return invalid("the message has %d parts; Discord allows %d (remove some blocks, links or buttons)", n, MaxComponents)
	}
	if n := d.TextLength() + reserve.Text; n > MaxTextTotal {
		return invalid("the message has %d characters of text; Discord allows %d", n, MaxTextTotal)
	}
	return nil
}

func (b *Block) validate(n int) error {
	clear := func() {
		if b.Type != BlockText && b.Type != BlockSection {
			b.Text = ""
		}
		if b.Type != BlockSection {
			b.Thumbnail = ""
		}
		if b.Type != BlockGallery {
			b.Images = nil
		}
		if b.Type != BlockSeparator {
			b.Divider, b.Large = false, false
		}
		if b.Type != BlockLinks {
			b.Links = nil
		}
	}
	switch b.Type {
	case BlockText:
		if b.Text == "" {
			return invalid("block %d: text is empty", n)
		}
	case BlockSection:
		if b.Text == "" {
			return invalid("block %d: text is empty", n)
		}
		if err := checkImageURL(b.Thumbnail); err != nil {
			return invalid("block %d: thumbnail %v", n, err)
		}
	case BlockGallery:
		if len(b.Images) == 0 || len(b.Images) > MaxGalleryItems {
			return invalid("block %d: a gallery has 1 to %d images", n, MaxGalleryItems)
		}
		for i, img := range b.Images {
			if err := checkImageURL(img.URL); err != nil {
				return invalid("block %d, image %d: %v", n, i+1, err)
			}
			if utf8.RuneCountInString(img.Description) > MaxAltText {
				return invalid("block %d, image %d: description is too long", n, i+1)
			}
		}
	case BlockSeparator:
	case BlockLinks:
		if len(b.Links) == 0 || len(b.Links) > MaxLinks {
			return invalid("block %d: a link row has 1 to %d links", n, MaxLinks)
		}
		for i := range b.Links {
			l := &b.Links[i]
			l.Label = strings.TrimSpace(l.Label)
			if l.Label == "" && l.Emoji == "" {
				return invalid("block %d, link %d: needs a label or an emoji", n, i+1)
			}
			if utf8.RuneCountInString(l.Label) > MaxLinkLabel {
				return invalid("block %d, link %d: label is longer than %d characters", n, i+1, MaxLinkLabel)
			}
			if err := checkLinkURL(l.URL); err != nil {
				return invalid("block %d, link %d: %v", n, i+1, err)
			}
			if l.Emoji != "" && !IsEmoji(l.Emoji) {
				return invalid("block %d, link %d: emoji must be a single Unicode emoji", n, i+1)
			}
		}
	default:
		return invalid("block %d: unknown type %q", n, b.Type)
	}
	clear()
	return nil
}

func checkImageURL(raw string) error {
	if raw == "" {
		return errors.New("is missing")
	}
	return checkURL(raw, "https")
}

func checkLinkURL(raw string) error {
	if raw == "" {
		return errors.New("URL is missing")
	}
	return checkURL(raw, "https", "http", "discord")
}

func checkURL(raw string, schemes ...string) error {
	if len(raw) > MaxURL {
		return errors.New("URL is too long")
	}
	u, err := url.Parse(raw)
	if err != nil || !slices.Contains(schemes, u.Scheme) || (u.Scheme != "discord" && u.Host == "") || u.User != nil {
		return fmt.Errorf("must be a %s URL", strings.Join(schemes, " or "))
	}
	return nil
}

// IsEmoji reports whether s is one Unicode emoji, possibly with skin-tone
// modifiers, variation selectors, joiners (family, profession emoji), tag
// sequences (flags of regions) or a keycap. Discord rejects anything else.
func IsEmoji(s string) bool {
	if s == "" || utf8.RuneCountInString(s) > 16 {
		return false
	}
	runes := []rune(s)
	// Keycaps: 0-9, # or *, an optional variation selector, then U+20E3.
	if r := runes[0]; (r >= '0' && r <= '9') || r == '#' || r == '*' {
		rest := runes[1:]
		if len(rest) > 0 && rest[0] == 0xFE0F {
			rest = rest[1:]
		}
		return len(rest) == 1 && rest[0] == 0x20E3
	}
	pictographs := 0
	for _, r := range runes {
		switch {
		case isPictograph(r):
			pictographs++
		case r == 0x200D, r == 0xFE0F, r == 0xFE0E, r >= 0xE0020 && r <= 0xE007F:
			// joiner, variation selectors, tags
		default:
			return false
		}
	}
	return pictographs > 0
}

func isPictograph(r rune) bool {
	switch {
	case r >= 0x1F000 && r <= 0x1FAFF: // emoji blocks, incl. skin tones and regional indicators
		return true
	case r >= 0x2600 && r <= 0x27BF, r >= 0x2300 && r <= 0x23FF, r >= 0x2B00 && r <= 0x2BFF,
		r >= 0x2190 && r <= 0x21FF, r >= 0x25A0 && r <= 0x25FF, r >= 0x2934 && r <= 0x2935:
		return true
	}
	switch r {
	case 0x00A9, 0x00AE, 0x203C, 0x2049, 0x2122, 0x2139, 0x24C2, 0x3030, 0x303D, 0x3297, 0x3299:
		return true
	}
	return false
}

// ---- variables -----------------------------------------------------------------

// Vars are the values variables render to. Mentions are already formatted
// by the platform (e.g. <@123>); names are escaped when rendered.
type Vars struct {
	UserMention string // {user}
	UserName    string // {user.name}
	UserID      string // {user.id}
	Server      string // {server}
	MemberCount int    // {memberCount}
	Channel     string // {channel} (a mention)
}

// VariableNames are the variables a document may use.
var VariableNames = []string{"user", "user.name", "user.id", "server", "memberCount", "channel"}

var varRE = regexp.MustCompile(`\{([A-Za-z][A-Za-z.]*)\}`)

// UnknownVariables lists variables in the document that Niro doesn't know
// (they are left as typed), so the editor can flag them.
func (d Document) UnknownVariables() []string {
	var out []string
	for _, b := range d.Blocks {
		for _, m := range varRE.FindAllStringSubmatch(b.Text, -1) {
			if !slices.Contains(VariableNames, m[1]) && !slices.Contains(out, m[1]) {
				out = append(out, m[1])
			}
		}
	}
	return out
}

// Render returns a copy of the document with variables replaced in text
// blocks. escape is the platform's Markdown escaper for names.
func (d Document) Render(v Vars, escape func(string) string) Document {
	values := map[string]string{
		"user": v.UserMention, "user.name": escape(v.UserName), "user.id": v.UserID,
		"server": escape(v.Server), "memberCount": fmt.Sprint(v.MemberCount), "channel": v.Channel,
	}
	out := d
	out.Blocks = slices.Clone(d.Blocks)
	for i := range out.Blocks {
		out.Blocks[i].Text = varRE.ReplaceAllStringFunc(out.Blocks[i].Text, func(m string) string {
			if val, ok := values[m[1:len(m)-1]]; ok {
				return val
			}
			return m
		})
	}
	return out
}

// Simple returns a document with one text block (defaults and tests).
func Simple(text string) Document {
	return Document{Version: Version, Blocks: []Block{{Type: BlockText, Text: text}}}
}
