package message

import (
	"encoding/json"
	"html"
	"testing"
	"unicode/utf8"
)

// FuzzDocument feeds arbitrary JSON through the path a dashboard document
// takes: decode, validate, render. None of it may panic, and rendering must
// not change the document's shape.
func FuzzDocument(f *testing.F) {
	f.Add(`{"version":1,"blocks":[{"type":"text","text":"Hi {user}"}]}`)
	f.Add(`{"version":1,"accentColor":16777215,"blocks":[{"type":"section","text":"x","thumbnail":"https://e.x/a.png"},{"type":"separator"},{"type":"gallery","images":[{"url":"https://e.x/b.png"}]},{"type":"links","links":[{"label":"Go","url":"https://e.x","emoji":"🔥"}]}]}`)
	f.Add(`{"blocks":[{"type":"text","text":"{memberCount}{server}{channel}{user.name}{nope}"}]}`)
	f.Fuzz(func(t *testing.T, raw string) {
		var d Document
		if json.Unmarshal([]byte(raw), &d) != nil {
			return
		}
		valid := d.Validate(Budget{}) == nil
		_ = d.UnknownVariables()
		out := d.Render(Vars{UserMention: "<@1>", UserName: "a", UserID: "1", Server: "s", MemberCount: 3, Channel: "<#2>"}, html.EscapeString)
		if valid && out.Components() != d.Components() {
			t.Fatalf("rendering changed the component count: %d -> %d", d.Components(), out.Components())
		}
	})
}

func FuzzIsEmoji(f *testing.F) {
	for _, s := range []string{"🔥", "👍🏽", "1️⃣", "🏳️‍🌈", "🇧🇪", "a", "", "<:x:1>", "🔥🔥"} {
		f.Add(s)
	}
	f.Fuzz(func(t *testing.T, s string) {
		if IsEmoji(s) && !utf8.ValidString(s) {
			t.Fatalf("invalid UTF-8 accepted as an emoji: %q", s)
		}
	})
}
