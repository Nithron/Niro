package guardian

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"strings"

	"github.com/disgoorg/snowflake/v2"

	"github.com/nithron/niro/internal/core"
)

// Mode decides whether Guardian acts or only reports.
type Mode string

const (
	// ModeEnforce deletes, quarantines and triggers raid mode.
	ModeEnforce Mode = "enforce"
	// ModeMonitor only posts alerts: useful to tune thresholds first.
	ModeMonitor Mode = "monitor"
)

// QuarantineMode is how a quarantined member is restricted.
type QuarantineMode string

const (
	QuarantineTimeout QuarantineMode = "timeout" // Discord timeout (max 28 days)
	QuarantineRole    QuarantineMode = "role"    // replace their roles with a quarantine role
)

// AgeAction is what happens to an account younger than the minimum age.
type AgeAction string

const (
	AgeAlert      AgeAction = "alert"
	AgeKick       AgeAction = "kick"
	AgeQuarantine AgeAction = "quarantine"
)

// Settings are the per-server Guardian settings.
type Settings struct {
	Mode Mode `json:"mode"`
	// AlertChannelID receives Guardian alerts; 0 uses the moderation log channel.
	AlertChannelID snowflake.ID   `json:"alert_channel_id"`
	ExemptRoleIDs  []snowflake.ID `json:"exempt_role_ids"`

	// Compromised accounts: the same message or link in BurstChannels
	// channels within BurstWindowSeconds.
	BurstEnabled       bool `json:"burst_enabled"`
	BurstChannels      int  `json:"burst_channels"`
	BurstWindowSeconds int  `json:"burst_window_seconds"`
	// QuietDays: a member silent for this long who suddenly posts a link is
	// suspicious (one weak signal). 0 turns the check off.
	QuietDays int `json:"quiet_days"`

	// Scam links: blocklist, look-alike domains, masked links and scam texts.
	ScamLinksEnabled bool     `json:"scam_links_enabled"`
	AllowDomains     []string `json:"allow_domains"`
	DenyDomains      []string `json:"deny_domains"`

	QuarantineMode   QuarantineMode `json:"quarantine_mode"`
	QuarantineRoleID snowflake.ID   `json:"quarantine_role_id"`
	QuarantineHours  int            `json:"quarantine_hours"`
	// DMUser sends quarantined members recovery steps.
	DMUser bool `json:"dm_user"`

	// Anti-raid: RaidJoins joins within RaidWindowSeconds start raid mode.
	RaidEnabled           bool `json:"raid_enabled"`
	RaidJoins             int  `json:"raid_joins"`
	RaidWindowSeconds     int  `json:"raid_window_seconds"`
	RaidDurationMinutes   int  `json:"raid_duration_minutes"`
	RaidPauseInvites      bool `json:"raid_pause_invites"`
	RaidRaiseVerification bool `json:"raid_raise_verification"`
	RaidSlowmodeSeconds   int  `json:"raid_slowmode_seconds"`

	// Account-age gate on join. 0 turns it off.
	MinAccountAgeHours int       `json:"min_account_age_hours"`
	AccountAgeAction   AgeAction `json:"account_age_action"`

	// Verification gate: the button grants VerifyRoleID to accounts at least
	// VerifyMinAccountAgeHours old.
	VerifyRoleID             snowflake.ID `json:"verify_role_id"`
	VerifyMinAccountAgeHours int          `json:"verify_min_account_age_hours"`
}

// Limits for settings and lists.
const (
	MaxDomainList    = 200
	MaxExemptRoles   = 25
	MaxQuarantineHrs = 28 * 24 // Discord's timeout limit
	MaxAccountAgeHrs = 365 * 24
	MaxSlowmode      = 21600 // Discord's limit (6 hours)
	MaxRaidMinutes   = 24 * 60
)

// DefaultSettings are conservative: they catch the common attacks without
// touching normal chat.
func DefaultSettings() Settings {
	return Settings{
		Mode:                  ModeEnforce,
		ExemptRoleIDs:         []snowflake.ID{},
		BurstEnabled:          true,
		BurstChannels:         3,
		BurstWindowSeconds:    60,
		QuietDays:             30,
		ScamLinksEnabled:      true,
		AllowDomains:          []string{},
		DenyDomains:           []string{},
		QuarantineMode:        QuarantineTimeout,
		QuarantineHours:       7 * 24,
		DMUser:                true,
		RaidEnabled:           true,
		RaidJoins:             10,
		RaidWindowSeconds:     30,
		RaidDurationMinutes:   30,
		RaidPauseInvites:      true,
		RaidRaiseVerification: true,
		AccountAgeAction:      AgeKick,
	}
}

// ErrInvalidSettings wraps every validation failure.
var ErrInvalidSettings = errors.New("invalid guardian settings")

func invalid(format string, a ...any) error {
	return fmt.Errorf("%w: %s", ErrInvalidSettings, fmt.Sprintf(format, a...))
}

// SettingsSpec validates Guardian settings for the config service.
type SettingsSpec struct{}

var _ core.SettingsSpec = SettingsSpec{}

func (SettingsSpec) Normalize(raw []byte) ([]byte, error) {
	s := DefaultSettings()
	if len(raw) > 0 {
		dec := json.NewDecoder(bytes.NewReader(raw))
		dec.DisallowUnknownFields()
		if err := dec.Decode(&s); err != nil {
			return nil, invalid("%v", err)
		}
	}
	if err := s.validate(); err != nil {
		return nil, err
	}
	return json.Marshal(s)
}

func (s *Settings) validate() error {
	inRange := func(name string, v, lo, hi int) error {
		if v < lo || v > hi {
			return invalid("%s must be between %d and %d", name, lo, hi)
		}
		return nil
	}
	if s.Mode != ModeEnforce && s.Mode != ModeMonitor {
		return invalid("mode must be enforce or monitor")
	}
	if s.QuarantineMode != QuarantineTimeout && s.QuarantineMode != QuarantineRole {
		return invalid("quarantine_mode must be timeout or role")
	}
	if s.QuarantineMode == QuarantineRole && s.QuarantineRoleID == 0 {
		return invalid("quarantine_role_id is required when quarantine_mode is role")
	}
	if s.AccountAgeAction != AgeAlert && s.AccountAgeAction != AgeKick && s.AccountAgeAction != AgeQuarantine {
		return invalid("account_age_action must be alert, kick or quarantine")
	}
	for _, err := range []error{
		inRange("burst_channels", s.BurstChannels, 2, 10),
		inRange("burst_window_seconds", s.BurstWindowSeconds, 10, 600),
		inRange("quiet_days", s.QuietDays, 0, 365),
		inRange("quarantine_hours", s.QuarantineHours, 1, MaxQuarantineHrs),
		inRange("raid_joins", s.RaidJoins, 3, 200),
		inRange("raid_window_seconds", s.RaidWindowSeconds, 5, 600),
		inRange("raid_duration_minutes", s.RaidDurationMinutes, 5, MaxRaidMinutes),
		inRange("raid_slowmode_seconds", s.RaidSlowmodeSeconds, 0, MaxSlowmode),
		inRange("min_account_age_hours", s.MinAccountAgeHours, 0, MaxAccountAgeHrs),
		inRange("verify_min_account_age_hours", s.VerifyMinAccountAgeHours, 0, MaxAccountAgeHrs),
	} {
		if err != nil {
			return err
		}
	}
	if s.QuietDays > 0 && s.QuietDays < 7 {
		return invalid("quiet_days must be 0 (off) or at least 7")
	}
	if s.ExemptRoleIDs == nil {
		s.ExemptRoleIDs = []snowflake.ID{}
	}
	if len(s.ExemptRoleIDs) > MaxExemptRoles {
		return invalid("at most %d exempt roles", MaxExemptRoles)
	}
	var err error
	if s.AllowDomains, err = normalizeDomains("allow_domains", s.AllowDomains); err != nil {
		return err
	}
	if s.DenyDomains, err = normalizeDomains("deny_domains", s.DenyDomains); err != nil {
		return err
	}
	for _, d := range s.DenyDomains {
		if slices.Contains(s.AllowDomains, d) {
			return invalid("%s is in both the allow and deny lists", d)
		}
	}
	return nil
}

func normalizeDomains(field string, in []string) ([]string, error) {
	out := []string{}
	for _, d := range in {
		d = strings.TrimSpace(d)
		if d == "" {
			continue
		}
		// Accept pasted URLs ("https://example.com/x") as well as bare domains.
		if i := strings.Index(d, "://"); i >= 0 {
			d = d[i+3:]
		}
		if i := strings.IndexAny(d, "/?#"); i >= 0 {
			d = d[:i]
		}
		h, ok := NormalizeHost(d)
		if !ok {
			return nil, invalid("%s: %q is not a domain", field, d)
		}
		if !slices.Contains(out, h) {
			out = append(out, h)
		}
	}
	if len(out) > MaxDomainList {
		return nil, invalid("%s: at most %d domains", field, MaxDomainList)
	}
	slices.Sort(out)
	return out, nil
}

// LoadSettings reads a server's Guardian settings.
func LoadSettings(ctx context.Context, cfg *core.ConfigService, guildID snowflake.ID) (Settings, error) {
	raw, err := cfg.Settings(ctx, guildID, core.ModuleGuardian)
	if err != nil {
		return Settings{}, err
	}
	var s Settings
	return s, json.Unmarshal(raw, &s)
}
