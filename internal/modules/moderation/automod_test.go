package moderation

import (
	"errors"
	"strings"
	"testing"

	"github.com/disgoorg/snowflake/v2"
)

func TestAutoModPresetsAreValid(t *testing.T) {
	for _, p := range AutoModPresets() {
		r := p.Rule
		if err := r.Normalize(); err != nil {
			t.Errorf("preset %s invalid: %v", p.ID, err)
		}
	}
	if _, ok := AutoModPresetByID("invite_links"); !ok {
		t.Fatal("invite_links preset missing")
	}
	if _, ok := AutoModPresetByID("nope"); ok {
		t.Fatal("unknown preset found")
	}
}

func TestAutoModNormalize(t *testing.T) {
	ok := func() AutoModRule {
		return AutoModRule{Name: " Bad words ", Trigger: TriggerKeyword, Keywords: []string{" a ", "a", "", "b"}, BlockMessage: true}
	}
	r := ok()
	if err := r.Normalize(); err != nil {
		t.Fatal(err)
	}
	if r.Name != "Bad words" || strings.Join(r.Keywords, ",") != "a,b" {
		t.Fatalf("not cleaned: %+v", r)
	}

	bad := map[string]func(*AutoModRule){
		"no name":          func(r *AutoModRule) { r.Name = " " },
		"unknown trigger":  func(r *AutoModRule) { r.Trigger = "magic" },
		"no keywords":      func(r *AutoModRule) { r.Keywords = nil },
		"long keyword":     func(r *AutoModRule) { r.Keywords = []string{strings.Repeat("x", 61)} },
		"bad regex":        func(r *AutoModRule) { r.RegexPatterns = []string{"(unclosed"} },
		"no action":        func(r *AutoModRule) { r.BlockMessage = false },
		"long message":     func(r *AutoModRule) { r.CustomMessage = strings.Repeat("x", 151) },
		"short timeout":    func(r *AutoModRule) { r.TimeoutSeconds = 5 },
		"spam timeout":     func(r *AutoModRule) { r.Trigger, r.TimeoutSeconds = TriggerSpam, 600 },
		"no preset":        func(r *AutoModRule) { r.Trigger = TriggerKeywordPreset },
		"unknown preset":   func(r *AutoModRule) { r.Trigger, r.Presets = TriggerKeywordPreset, []string{"rude"} },
		"mention limit 0":  func(r *AutoModRule) { r.Trigger = TriggerMentionSpam },
		"too many exempts": func(r *AutoModRule) { r.ExemptRoleIDs = make([]snowflake.ID, 21) },
	}
	for name, mutate := range bad {
		r := ok()
		mutate(&r)
		if err := r.Normalize(); !errors.Is(err, ErrInvalidRule) {
			t.Errorf("%s: err = %v, want ErrInvalidRule", name, err)
		}
	}

	// Fields that don't apply to the trigger are cleared.
	r = AutoModRule{Name: "x", Trigger: TriggerSpam, Keywords: []string{"k"}, MentionLimit: 5, BlockMessage: true, CustomMessage: "hi"}
	if err := r.Normalize(); err != nil || r.Keywords != nil || r.MentionLimit != 0 {
		t.Fatalf("irrelevant fields kept: %+v, %v", r, err)
	}
}
