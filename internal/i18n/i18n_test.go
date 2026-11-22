package i18n

import (
	"strings"
	"testing"
	"testing/fstest"

	"github.com/nithron/nithro/internal/core"
	"github.com/nithron/nithro/internal/modules/moderation"
	"golang.org/x/text/language"
)

func TestEnglishCatalogIsComplete(t *testing.T) {
	c, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	if missing := c.Missing(language.English); len(missing) > 0 {
		t.Fatalf("en.toml is missing keys: %v", missing)
	}
	for _, a := range moderation.Actions {
		if k := ModAction(string(a)); c.T("en", k, nil) == string(k) {
			t.Errorf("en.toml is missing %s", k)
		}
	}
	for _, m := range core.DefaultModules() {
		for _, k := range []Key{ModuleName(string(m.ID)), ModuleDescription(string(m.ID))} {
			if c.T("en", k, nil) == string(k) {
				t.Errorf("en.toml is missing %s", k)
			}
		}
	}
}

func TestOtherLanguagesReportUntranslatedKeys(t *testing.T) {
	c, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	for _, tag := range c.Languages() {
		if tag == language.English {
			continue
		}
		if missing := c.Missing(tag); len(missing) > 0 {
			// Not fatal: untranslated keys fall back to English.
			t.Logf("%s: %d untranslated keys (falls back to English)", tag, len(missing))
		}
	}
}

func TestTranslateFallbackAndTemplates(t *testing.T) {
	fsys := fstest.MapFS{
		"l/en.toml": {Data: []byte(`"a" = "Hello {{.Name}}"` + "\n" + `"b" = "Only English"`)},
		"l/nl.toml": {Data: []byte(`"a" = "Hallo {{.Name}}"`)},
	}
	c, err := LoadFS(fsys, "l")
	if err != nil {
		t.Fatal(err)
	}
	cases := []struct {
		locale string
		key    Key
		want   string
	}{
		{"en-US", "a", "Hello Ana"},
		{"nl", "a", "Hallo Ana"},
		{"nl", "b", "Only English"}, // untranslated falls back
		{"fr", "a", "Hello Ana"},    // unknown language falls back
		{"en", "missing.key", "missing.key"},
	}
	for _, tc := range cases {
		if got := c.T(tc.locale, tc.key, map[string]any{"Name": "Ana"}); got != tc.want {
			t.Errorf("T(%s, %s) = %q, want %q", tc.locale, tc.key, got, tc.want)
		}
	}
	tr := c.Translations("a")
	if got, ok := tr[language.Dutch]; len(tr) != 1 || !ok || !strings.HasPrefix(got, "Hallo") {
		t.Errorf("Translations(a) = %v", tr)
	}
	if len(c.Translations("b")) != 0 {
		t.Error("untranslated key must not produce a localization")
	}
}

func TestLoadRequiresEnglish(t *testing.T) {
	fsys := fstest.MapFS{"l/nl.toml": {Data: []byte(`"a" = "x"`)}}
	if _, err := LoadFS(fsys, "l"); err == nil {
		t.Fatal("catalog without en.toml accepted")
	}
}
