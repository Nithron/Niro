package guardian

import (
	"slices"
	"testing"
)

func TestExtractLinks(t *testing.T) {
	links := ExtractLinks("hey [discord.com/gift](https://dlscord.gift/x) and https://WWW.Example.com/path?q=1, also <https://foo.bar.co.uk/>")
	if len(links) != 3 {
		t.Fatalf("got %d links: %+v", len(links), links)
	}
	if !links[0].Masked || links[0].Text != "discord.com/gift" || links[0].Host != "dlscord.gift" {
		t.Errorf("masked link = %+v", links[0])
	}
	if links[1].Host != "example.com" || links[1].Domain != "example.com" {
		t.Errorf("bare link = %+v", links[1])
	}
	if links[2].Host != "foo.bar.co.uk" || links[2].Domain != "bar.co.uk" {
		t.Errorf("co.uk link = %+v", links[2])
	}
}

func TestExtractLinksIgnoresJunk(t *testing.T) {
	for _, s := range []string{"no links here", "ftp://x.com", "http://127.0.0.1/a", "https://localhost/", "www.example.com"} {
		if l := ExtractLinks(s); len(l) != 0 {
			t.Errorf("%q: unexpected links %+v", s, l)
		}
	}
}

func TestPunycodeIsDecoded(t *testing.T) {
	// xn--dscord-pvf.com is "dіscord.com" with a Cyrillic і.
	l := ExtractLinks("https://xn--dscord-pvf.com/nitro")
	if len(l) != 1 {
		t.Fatalf("links = %+v", l)
	}
	if Lookalike(l[0].Host) != "discord" {
		t.Errorf("host %q not flagged as a discord look-alike", l[0].Host)
	}
}

func TestLookalike(t *testing.T) {
	flagged := []string{
		"dlscord.com", "discrod.gift", "dicsord.com", "discord-nitro.gift", "free-nitro.xyz",
		"discordgift.site", "steamcommunlty.com", "stearncommunity.com", "discord.com.evil.ru",
		"steamcommunity.com.login.ru", "d1scord.gg", "discordapp.co", "dіscord.com",
	}
	for _, h := range flagged {
		if Lookalike(h) == "" {
			t.Errorf("%s: not flagged", h)
		}
	}
	safe := []string{
		"discord.com", "cdn.discordapp.com", "discord.gg", "media.discordapp.net", "steamcommunity.com",
		"store.steampowered.com", "discords.com", "discord.mycommunity.org", "discordbotlist.com",
		"steamgifts.com", "nitrotype.com", "github.com", "youtube.com", "disco.com", "discord.js.org",
		"top.gg", "roblox.com", "twitch.tv",
	}
	for _, h := range safe {
		if b := Lookalike(h); b != "" {
			t.Errorf("%s: wrongly flagged as %s", h, b)
		}
	}
}

func TestDomainSetMatchesParents(t *testing.T) {
	s := NewDomainSet("evil.ru", "Bad.Example.COM", "not a domain")
	if s.Len() != 2 {
		t.Fatalf("len = %d", s.Len())
	}
	cases := map[string]string{"evil.ru": "evil.ru", "a.b.evil.ru": "evil.ru", "bad.example.com": "bad.example.com", "example.com": "", "notevil.ru": ""}
	for host, want := range cases {
		if got := s.Match(host); got != want {
			t.Errorf("Match(%s) = %q, want %q", host, got, want)
		}
	}
}

func TestMaskedMismatch(t *testing.T) {
	cases := []struct {
		content string
		want    bool
	}{
		{"[discord.com/nitro](https://evil.ru/x)", true},
		{"[https://steamcommunity.com/gift](https://steamcommunity.ru/gift)", true},
		{"[click here](https://evil.ru)", false},            // no domain shown
		{"[docs](https://example.com/docs)", false},         // no domain shown
		{"[example.com](https://www.example.com/a)", false}, // same site
		{"[v1.2 notes](https://example.com)", false},        // not a real domain
	}
	for _, c := range cases {
		links := ExtractLinks(c.content)
		if len(links) != 1 {
			t.Fatalf("%q: links = %+v", c.content, links)
		}
		if _, got := maskedMismatch(links[0]); got != c.want {
			t.Errorf("%q: mismatch = %v, want %v", c.content, got, c.want)
		}
	}
}

func TestTemplates(t *testing.T) {
	cases := []struct {
		text    string
		hasLink bool
		want    []string
	}{
		{"FR3E N1TRO for everyone, claim now", true, []string{"free_nitro"}},
		{"Free Discord Nitrо giveaway!!", false, []string{"free_nitro"}}, // Cyrillic о
		{"I got a $50 steam gift card, first come first serve", true, []string{"steam_gift"}},
		{"bro i accidentally reported your account, talk to this admin", false, []string{"false_report"}},
		{"Huge AIRDROP live, claim your tokens", true, []string{"crypto_airdrop"}},
		{"@everyone check this out", true, []string{"mass_ping_link"}},
		{"@everyone meeting in 5", false, nil},
		{"does anyone know if nitro is worth it?", false, nil},
		{"I bought a game on steam yesterday", false, nil},
		{"please report bugs in #support", false, nil},
	}
	for _, c := range cases {
		got := matchTemplates(c.text, c.hasLink)
		if !slices.Equal(got, c.want) {
			t.Errorf("%q: templates = %v, want %v", c.text, got, c.want)
		}
	}
}

func TestNormalizeTextMakesNearDuplicatesEqual(t *testing.T) {
	a := normalizeText("Hey <@123>!! Check https://evil.ru/abc?ref=1 NOW")
	b := normalizeText("hey <@456> check   https://evil.ru/zzz now")
	if a != b {
		t.Errorf("%q != %q", a, b)
	}
}

func TestDamerau(t *testing.T) {
	cases := map[[2]string]int{{"discord", "discord"}: 0, {"discrod", "discord"}: 1, {"dixcord", "discord"}: 1, {"discor", "discord"}: 1, {"dsicrod", "discord"}: 2}
	for in, want := range cases {
		if got := damerau(in[0], in[1]); got != want {
			t.Errorf("damerau(%q,%q) = %d, want %d", in[0], in[1], got, want)
		}
	}
}

func TestParseFeed(t *testing.T) {
	for name, body := range map[string]string{
		"object": `{"domains": ["evil.ru", "EVIL.ru", "discord.com", "x"]}`,
		"array":  `["evil.ru", "discord.gg"]`,
		"text":   "# comment\nevil.ru\n\ndiscord.com 0.0.0.0\n",
	} {
		got, err := ParseFeed([]byte(body))
		if err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		// Discord's own domains are never accepted from a feed.
		if !slices.Equal(got, []string{"evil.ru"}) {
			t.Errorf("%s: %v", name, got)
		}
	}
	if _, err := ParseFeed([]byte(`{"domains": `)); err == nil {
		t.Error("broken JSON accepted")
	}
}
