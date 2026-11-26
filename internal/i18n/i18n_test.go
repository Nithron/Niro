package i18n

import (
	"strings"
	"testing"
	"testing/fstest"

	"github.com/nithron/niro/internal/core"
	"github.com/nithron/niro/internal/modules/guardian"
	"github.com/nithron/niro/internal/modules/logging"
	"github.com/nithron/niro/internal/modules/moderation"
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
	dynamic := []Key{}
	for _, k := range []guardian.SignalKind{guardian.SignalBurst, guardian.SignalBlockedDomain, guardian.SignalDeniedDomain,
		guardian.SignalLookalike, guardian.SignalMaskedLink, guardian.SignalScamTemplate, guardian.SignalQuietAccount,
		guardian.SignalNewAccount, guardian.SignalJoinFlood, guardian.SignalManual} {
		dynamic = append(dynamic, GuardianSignal(string(k)))
	}
	for _, r := range []guardian.Response{guardian.ResponseAlert, guardian.ResponseDelete, guardian.ResponseQuarantine,
		guardian.ResponseKick, guardian.ResponseRaidMode} {
		dynamic = append(dynamic, GuardianAction(string(r)))
	}
	for _, s := range []guardian.Status{guardian.StatusOpen, guardian.StatusKept, guardian.StatusRestored,
		guardian.StatusBanned, guardian.StatusActive, guardian.StatusEnded} {
		dynamic = append(dynamic, GuardianStatus(string(s)))
	}
	for _, k := range []guardian.Kind{guardian.KindCompromised, guardian.KindScam, guardian.KindRaid, guardian.KindAccountAge} {
		dynamic = append(dynamic, GuardianTitle(string(k)))
	}
	for _, p := range guardian.DangerousPermissions {
		dynamic = append(dynamic, GuardianPerm(p))
	}
	for _, code := range []string{guardian.FindEveryoneDangerous, guardian.FindRoleAdmin, guardian.FindRoleDangerous,
		guardian.FindBotAdmin, guardian.FindBotDangerous, guardian.FindWebhooks, guardian.FindVerificationNone,
		guardian.FindVerificationLow, guardian.FindNoModeratorMFA, guardian.FindSelfAdmin} {
		dynamic = append(dynamic, GuardianAudit(code))
	}
	for _, s := range []guardian.Severity{guardian.SeverityCritical, guardian.SeverityHigh, guardian.SeverityMedium, guardian.SeverityInfo} {
		dynamic = append(dynamic, GuardianSeverity(string(s)))
	}
	for _, p := range guardian.Problems {
		dynamic = append(dynamic, GuardianProblem(p))
	}
	for _, k := range logging.Kinds {
		dynamic = append(dynamic, LogTitle(string(k)))
	}
	for _, a := range []string{"block_message", "send_alert", "timeout", "block_interaction"} {
		dynamic = append(dynamic, LogAutoModAction(a))
	}
	for _, s := range []string{"guardian", "purge"} {
		dynamic = append(dynamic, LogNiroSource(s))
	}
	for _, k := range dynamic {
		if c.T("en", k, nil) == string(k) {
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
