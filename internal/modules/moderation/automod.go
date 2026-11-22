package moderation

import (
	"errors"
	"fmt"
	"regexp"
	"slices"
	"strings"
	"unicode/utf8"

	"github.com/disgoorg/snowflake/v2"
)

// AutoModRule is a simplified, validated view of a native AutoMod rule (the
// platform enforces it; Nithro only manages it). JSON field names are the
// dashboard API's.
type AutoModRule struct {
	ID      snowflake.ID `json:"id,omitempty"`
	Name    string       `json:"name"`
	Enabled bool         `json:"enabled"`
	// Trigger: keyword | spam | keyword_preset | mention_spam
	Trigger               string   `json:"trigger"`
	Keywords              []string `json:"keywords"`
	RegexPatterns         []string `json:"regexPatterns"`
	AllowList             []string `json:"allowList"`
	Presets               []string `json:"presets"` // profanity | sexual_content | slurs
	MentionLimit          int      `json:"mentionLimit"`
	MentionRaidProtection bool     `json:"mentionRaidProtection"`

	BlockMessage   bool         `json:"blockMessage"`
	CustomMessage  string       `json:"customMessage"`
	AlertChannelID snowflake.ID `json:"alertChannelId,omitempty"`
	TimeoutSeconds int          `json:"timeoutSeconds"`

	ExemptRoleIDs    []snowflake.ID `json:"exemptRoleIds"`
	ExemptChannelIDs []snowflake.ID `json:"exemptChannelIds"`

	// CreatedByBot is true for rules Nithro created (read-only).
	CreatedByBot bool `json:"createdByBot"`
}

const (
	TriggerKeyword       = "keyword"
	TriggerSpam          = "spam"
	TriggerKeywordPreset = "keyword_preset"
	TriggerMentionSpam   = "mention_spam"
)

var (
	automodTriggers = []string{TriggerKeyword, TriggerSpam, TriggerKeywordPreset, TriggerMentionSpam}
	automodPresets  = []string{"profanity", "sexual_content", "slurs"}
)

// Discord's limits for AutoMod rules.
const (
	maxRuleName      = 100
	maxKeywords      = 1000
	maxKeywordLen    = 60
	maxRegex         = 10
	maxRegexLen      = 260
	maxAllowKeyword  = 100
	maxAllowPreset   = 1000
	maxCustomMessage = 150
	maxExempt        = 50
	maxAutoTimeout   = 28 * 24 * 60 * 60
)

// ErrInvalidRule wraps every validation failure.
var ErrInvalidRule = errors.New("invalid AutoMod rule")

func invalid(format string, args ...any) error {
	return fmt.Errorf("%w: %s", ErrInvalidRule, fmt.Sprintf(format, args...))
}

// Normalize trims and de-duplicates list fields, then validates the rule
// against Discord's limits. It returns a user-facing error on failure.
func (r *AutoModRule) Normalize() error {
	r.Name = strings.TrimSpace(r.Name)
	r.CustomMessage = strings.TrimSpace(r.CustomMessage)
	r.Keywords = cleanList(r.Keywords)
	r.RegexPatterns = cleanList(r.RegexPatterns)
	r.AllowList = cleanList(r.AllowList)
	r.Presets = cleanList(r.Presets)

	switch n := utf8.RuneCountInString(r.Name); {
	case n == 0:
		return invalid("the rule needs a name")
	case n > maxRuleName:
		return invalid("the name can be at most %d characters", maxRuleName)
	}
	if !slices.Contains(automodTriggers, r.Trigger) {
		return invalid("unknown trigger %q", r.Trigger)
	}

	switch r.Trigger {
	case TriggerKeyword:
		if len(r.Keywords) == 0 && len(r.RegexPatterns) == 0 {
			return invalid("add at least one keyword or pattern")
		}
		if len(r.Keywords) > maxKeywords {
			return invalid("at most %d keywords", maxKeywords)
		}
		for _, k := range r.Keywords {
			if utf8.RuneCountInString(k) > maxKeywordLen {
				return invalid("keywords can be at most %d characters (%q)", maxKeywordLen, k)
			}
		}
		if len(r.RegexPatterns) > maxRegex {
			return invalid("at most %d patterns", maxRegex)
		}
		for _, p := range r.RegexPatterns {
			if utf8.RuneCountInString(p) > maxRegexLen {
				return invalid("patterns can be at most %d characters", maxRegexLen)
			}
			if _, err := regexp.Compile(p); err != nil {
				return invalid("pattern %q is not valid: %v", p, err)
			}
		}
		if len(r.AllowList) > maxAllowKeyword {
			return invalid("at most %d allowed words", maxAllowKeyword)
		}
	case TriggerKeywordPreset:
		if len(r.Presets) == 0 {
			return invalid("choose at least one word list")
		}
		for _, p := range r.Presets {
			if !slices.Contains(automodPresets, p) {
				return invalid("unknown word list %q", p)
			}
		}
		if len(r.AllowList) > maxAllowPreset {
			return invalid("at most %d allowed words", maxAllowPreset)
		}
	case TriggerMentionSpam:
		if r.MentionLimit < 1 || r.MentionLimit > 50 {
			return invalid("the mention limit must be between 1 and 50")
		}
	}
	// Fields that don't apply to the trigger are cleared, not rejected.
	if r.Trigger != TriggerKeyword {
		r.Keywords, r.RegexPatterns = nil, nil
	}
	if r.Trigger != TriggerKeyword && r.Trigger != TriggerKeywordPreset {
		r.AllowList = nil
	}
	if r.Trigger != TriggerKeywordPreset {
		r.Presets = nil
	}
	if r.Trigger != TriggerMentionSpam {
		r.MentionLimit, r.MentionRaidProtection = 0, false
	}

	if !r.BlockMessage && r.AlertChannelID == 0 && r.TimeoutSeconds == 0 {
		return invalid("choose at least one action: block the message, send an alert, or time out")
	}
	if utf8.RuneCountInString(r.CustomMessage) > maxCustomMessage {
		return invalid("the block message can be at most %d characters", maxCustomMessage)
	}
	if !r.BlockMessage {
		r.CustomMessage = ""
	}
	if r.TimeoutSeconds != 0 {
		if r.Trigger != TriggerKeyword && r.Trigger != TriggerMentionSpam {
			return invalid("timeouts are only available for keyword and mention spam rules")
		}
		if r.TimeoutSeconds < 60 || r.TimeoutSeconds > maxAutoTimeout {
			return invalid("the timeout must be between 1 minute and 28 days")
		}
	}
	if len(r.ExemptRoleIDs) > 20 {
		return invalid("at most 20 exempt roles")
	}
	if len(r.ExemptChannelIDs) > maxExempt {
		return invalid("at most %d exempt channels", maxExempt)
	}
	return nil
}

func cleanList(in []string) []string {
	out := make([]string, 0, len(in))
	for _, s := range in {
		if s = strings.TrimSpace(s); s != "" && !slices.Contains(out, s) {
			out = append(out, s)
		}
	}
	return out
}

// AutoModPreset is a ready-made rule the dashboard can add with one click.
type AutoModPreset struct {
	ID   string      `json:"id"`
	Rule AutoModRule `json:"rule"`
}

// AutoModPresets returns Nithro's rule templates. Names are English; the
// dashboard shows translated titles by preset ID.
func AutoModPresets() []AutoModPreset {
	return []AutoModPreset{
		{ID: "invite_links", Rule: AutoModRule{
			Name: "Nithro · Block invite links", Enabled: true, Trigger: TriggerKeyword,
			RegexPatterns: []string{`(?i)(discord\.(gg|io|me|li)|discord(app)?\.com/invite)/[a-z0-9-]+`},
			BlockMessage:  true, CustomMessage: "Invite links aren't allowed here.",
		}},
		{ID: "scam_phrases", Rule: AutoModRule{
			Name: "Nithro · Common scam phrases", Enabled: true, Trigger: TriggerKeyword,
			Keywords: []string{
				"free nitro", "nitro giveaway", "steam gift", "free discord nitro", "claim your nitro",
				"airdrop claim", "*giving away $*", "@everyone free*", "crypto giveaway", "check my bio*",
			},
			BlockMessage: true, CustomMessage: "This message looks like a known scam and was blocked.",
		}},
		{ID: "mention_spam", Rule: AutoModRule{
			Name: "Nithro · Mention spam", Enabled: true, Trigger: TriggerMentionSpam,
			MentionLimit: 6, MentionRaidProtection: true, BlockMessage: true, TimeoutSeconds: 600,
		}},
		{ID: "spam", Rule: AutoModRule{
			Name: "Nithro · Suspected spam", Enabled: true, Trigger: TriggerSpam, BlockMessage: true,
		}},
		{ID: "word_lists", Rule: AutoModRule{
			Name: "Nithro · Slurs and explicit content", Enabled: true, Trigger: TriggerKeywordPreset,
			Presets: []string{"slurs", "sexual_content"}, BlockMessage: true,
		}},
	}
}

// AutoModPresetByID finds a preset.
func AutoModPresetByID(id string) (AutoModPreset, bool) {
	for _, p := range AutoModPresets() {
		if p.ID == id {
			return p, true
		}
	}
	return AutoModPreset{}, false
}
