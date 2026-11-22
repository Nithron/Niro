package moderation

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"

	"github.com/disgoorg/snowflake/v2"

	"github.com/nithron/niro/internal/core"
)

// Settings are the per-server moderation settings.
type Settings struct {
	// LogChannelID is where case log messages are posted; 0 disables the log.
	LogChannelID snowflake.ID `json:"log_channel_id"`
	// DMUsers sends the affected user a private message about warnings,
	// timeouts, kicks and bans (moderators can opt out per action).
	DMUsers bool `json:"dm_users"`
	// RequireReason makes the reason option mandatory.
	RequireReason bool `json:"require_reason"`
}

// DefaultSettings are used until a server saves its own.
func DefaultSettings() Settings { return Settings{DMUsers: true} }

// SettingsSpec validates moderation settings for the config service.
type SettingsSpec struct{}

var _ core.SettingsSpec = SettingsSpec{}

func (SettingsSpec) Normalize(raw []byte) ([]byte, error) {
	s := DefaultSettings()
	if len(raw) > 0 {
		dec := json.NewDecoder(bytes.NewReader(raw))
		dec.DisallowUnknownFields()
		if err := dec.Decode(&s); err != nil {
			return nil, fmt.Errorf("moderation settings: %w", err)
		}
	}
	return json.Marshal(s)
}

// LoadSettings reads a server's moderation settings.
func LoadSettings(ctx context.Context, cfg *core.ConfigService, guildID snowflake.ID) (Settings, error) {
	raw, err := cfg.Settings(ctx, guildID, core.ModuleModeration)
	if err != nil {
		return Settings{}, err
	}
	var s Settings
	return s, json.Unmarshal(raw, &s)
}
