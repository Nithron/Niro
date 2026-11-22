package discord

import (
	"context"
	"io"
	"log/slog"
	"strings"
	"testing"
	"testing/fstest"
	"time"

	"github.com/disgoorg/disgo/discord"
	"github.com/disgoorg/disgo/rest"
	"github.com/disgoorg/snowflake/v2"

	"github.com/nithron/nithro/internal/i18n"
)

func testBot(t *testing.T) *Bot {
	t.Helper()
	cat, err := i18n.Load()
	if err != nil {
		t.Fatal(err)
	}
	return &Bot{deps: Deps{Catalog: cat}}
}

func TestCommandsRespectDiscordLimits(t *testing.T) {
	cmds := testBot(t).commands()
	names := map[string]bool{}
	for _, c := range cmds {
		s, ok := c.(discord.SlashCommandCreate)
		if !ok {
			t.Fatalf("unexpected command type %T", c)
		}
		names[s.Name] = true
		if s.Name != strings.ToLower(s.Name) || len(s.Name) > 32 {
			t.Errorf("%s: name must be lowercase and ≤32 chars", s.Name)
		}
		if n := len([]rune(s.Description)); n == 0 || n > 100 {
			t.Errorf("%s: description length %d, must be 1–100", s.Name, n)
		}
		if len(s.Contexts) != 1 || s.Contexts[0] != discord.InteractionContextTypeGuild {
			t.Errorf("%s: must be guild-only in v0.1", s.Name)
		}
	}
	for _, want := range []string{"ping", "about", "modules"} {
		if !names[want] {
			t.Errorf("missing /%s", want)
		}
	}
}

func TestModulesCommandRequiresManageGuild(t *testing.T) {
	for _, c := range testBot(t).commands() {
		s := c.(discord.SlashCommandCreate)
		if s.Name != "modules" {
			continue
		}
		p := s.DefaultMemberPermissions.Or(nil)
		if s.DefaultMemberPermissions.IsZero() || p == nil || *p != discord.PermissionManageGuild {
			t.Fatalf("/modules default permissions = %v, want Manage Server", p)
		}
		return
	}
	t.Fatal("/modules not found")
}

func TestLocalizationsMapToDiscordLocales(t *testing.T) {
	cat, err := i18n.LoadFS(fstest.MapFS{
		"l/en.toml": {Data: []byte(`"k" = "Hello"`)},
		"l/nl.toml": {Data: []byte(`"k" = "Hallo"`)},
		"l/pt.toml": {Data: []byte(`"k" = "Olá"`)},
	}, "l")
	if err != nil {
		t.Fatal(err)
	}
	got := localizations(cat, "k")
	if got[discord.LocaleDutch] != "Hallo" {
		t.Errorf("nl = %q", got[discord.LocaleDutch])
	}
	if got[discord.LocalePortugueseBR] != "Olá" {
		t.Errorf("pt-BR should fall back to base language pt, got %q", got[discord.LocalePortugueseBR])
	}
	if _, ok := got[discord.LocaleEnglishUS]; ok {
		t.Error("English must not be listed as a localization (it is the default)")
	}
	if localizations(cat, "missing") != nil {
		t.Error("untranslated key should produce no localizations")
	}
}

func TestWithLinksUsesButtonsOnlyForPublicHTTPS(t *testing.T) {
	c := withLinks(discord.NewContainer(), link{"Dash", "http://localhost:8080"}, link{"Site", "https://nithron.com"})
	var text, buttons int
	for _, sub := range c.Components {
		switch v := sub.(type) {
		case discord.TextDisplayComponent:
			text++
			if !strings.Contains(v.Content, "[Dash](http://localhost:8080)") {
				t.Errorf("local URL should be a text link, got %q", v.Content)
			}
		case discord.ActionRowComponent:
			buttons += len(v.Components)
		}
	}
	if text != 1 || buttons != 1 {
		t.Fatalf("text=%d buttons=%d, want 1 and 1", text, buttons)
	}
	for raw, want := range map[string]bool{
		"https://app.nithro.gg/x": true, "http://example.com": false, "https://localhost": false,
		"https://127.0.0.1:8080": false, "::bad": false,
	} {
		if got := publicHTTPS(raw); got != want {
			t.Errorf("publicHTTPS(%q) = %v, want %v", raw, got, want)
		}
	}
}

func TestFormatLatency(t *testing.T) {
	if formatLatency(0) != "—" || formatLatency(42*time.Millisecond) != "42 ms" {
		t.Fatal("unexpected latency formatting")
	}
}

// fakeApps implements the emoji part of rest.Applications.
type fakeApps struct {
	rest.Applications
	existing []discord.Emoji
	created  []string
}

func (f *fakeApps) GetApplicationEmojis(snowflake.ID, ...rest.RequestOpt) ([]discord.Emoji, error) {
	return f.existing, nil
}

func (f *fakeApps) CreateApplicationEmoji(_ snowflake.ID, c discord.EmojiCreate, _ ...rest.RequestOpt) (*discord.Emoji, error) {
	f.created = append(f.created, c.Name)
	return &discord.Emoji{ID: snowflake.ID(1000 + len(f.created)), Name: c.Name}, nil
}

func TestEmojiSyncUploadsOnlyMissing(t *testing.T) {
	apps := &fakeApps{existing: []discord.Emoji{{ID: 5, Name: EmojiOK}}}
	e := NewEmojis(apps, 1, slog.New(slog.NewTextHandler(io.Discard, nil)))

	if got := e.Get(EmojiOK); got != "✅" {
		t.Fatalf("before sync Get(ok) = %q, want Unicode fallback", got)
	}
	if err := e.Sync(context.Background()); err != nil {
		t.Fatal(err)
	}
	for _, name := range apps.created {
		if name == EmojiOK {
			t.Fatal("existing emoji re-uploaded")
		}
	}
	if len(apps.created) != 6 {
		t.Fatalf("uploaded %v, want the 6 missing brand emojis", apps.created)
	}
	if got := e.Get(EmojiOK); got != "<:ok:5>" {
		t.Fatalf("Get(ok) = %q", got)
	}
	if got := e.Get(EmojiShield); !strings.HasPrefix(got, "<:shield:") {
		t.Fatalf("Get(shield) = %q", got)
	}
}
