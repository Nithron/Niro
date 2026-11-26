package logging

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"slices"

	"github.com/disgoorg/snowflake/v2"

	"github.com/nithron/niro/internal/core"
)

// Channels is the log channel per category; 0 uses DefaultChannelID.
type Channels struct {
	Messages   snowflake.ID `json:"messages"`
	Members    snowflake.ID `json:"members"`
	Moderation snowflake.ID `json:"moderation"`
	Server     snowflake.ID `json:"server"`
	Voice      snowflake.ID `json:"voice"`
	AutoMod    snowflake.ID `json:"automod"`
	Guardian   snowflake.ID `json:"guardian"`
}

// For returns the channel set for a category (0 if none).
func (c Channels) For(cat Category) snowflake.ID {
	switch cat {
	case CategoryMessages:
		return c.Messages
	case CategoryMembers:
		return c.Members
	case CategoryModeration:
		return c.Moderation
	case CategoryServer:
		return c.Server
	case CategoryVoice:
		return c.Voice
	case CategoryAutoMod:
		return c.AutoMod
	case CategoryGuardian:
		return c.Guardian
	}
	return 0
}

// All returns every channel that is set, without duplicates.
func (c Channels) All() []snowflake.ID {
	var out []snowflake.ID
	for _, cat := range Categories {
		if id := c.For(cat); id != 0 && !slices.Contains(out, id) {
			out = append(out, id)
		}
	}
	return out
}

// Settings are the per-server logging settings.
type Settings struct {
	// DefaultChannelID receives every category without a channel of its own;
	// 0 turns those categories off.
	DefaultChannelID snowflake.ID `json:"default_channel_id"`
	Channels         Channels     `json:"channels"`
	// Events turns single kinds on or off. Normalize fills in every kind.
	Events map[Kind]bool `json:"events"`

	// Ignore lists apply to message and voice events.
	IgnoreChannelIDs []snowflake.ID `json:"ignore_channel_ids"`
	IgnoreRoleIDs    []snowflake.ID `json:"ignore_role_ids"`
	// IgnoreBots skips messages and voice activity of bots. Bot joins and
	// leaves are always logged: they matter for security.
	IgnoreBots bool `json:"ignore_bots"`
	// NoMessageContent: never cache or show message text. Edits and deletes
	// are still logged, without their text.
	NoMessageContent bool `json:"no_message_content"`
}

// Limits.
const (
	MaxIgnoreChannels = 100
	MaxIgnoreRoles    = 25
)

// DefaultSettings log nothing until a channel is chosen.
func DefaultSettings() Settings {
	s := Settings{
		Events:           map[Kind]bool{},
		IgnoreChannelIDs: []snowflake.ID{},
		IgnoreRoleIDs:    []snowflake.ID{},
		IgnoreBots:       true,
	}
	for _, k := range kinds {
		s.Events[k.kind] = k.defaultOn
	}
	return s
}

// ErrInvalidSettings wraps every validation failure.
var ErrInvalidSettings = errors.New("invalid logging settings")

func invalid(format string, a ...any) error {
	return fmt.Errorf("%w: %s", ErrInvalidSettings, fmt.Sprintf(format, a...))
}

// SettingsSpec validates logging settings for the config service.
type SettingsSpec struct{}

var _ core.SettingsSpec = SettingsSpec{}

func (SettingsSpec) Normalize(raw []byte) ([]byte, error) {
	s := DefaultSettings()
	if len(raw) > 0 {
		dec := json.NewDecoder(bytes.NewReader(raw))
		dec.DisallowUnknownFields()
		// Decoding merges into the default Events map, so kinds a stored
		// document doesn't mention (added in a later release) get their default.
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
	if s.Events == nil {
		s.Events = DefaultSettings().Events
	}
	for k := range s.Events {
		if !k.Valid() {
			return invalid("unknown event %q", k)
		}
	}
	for _, k := range kinds {
		if _, ok := s.Events[k.kind]; !ok {
			s.Events[k.kind] = k.defaultOn
		}
	}
	var err error
	if s.IgnoreChannelIDs, err = uniqueIDs("ignore_channel_ids", s.IgnoreChannelIDs, MaxIgnoreChannels); err != nil {
		return err
	}
	if s.IgnoreRoleIDs, err = uniqueIDs("ignore_role_ids", s.IgnoreRoleIDs, MaxIgnoreRoles); err != nil {
		return err
	}
	for _, ch := range append(s.Channels.All(), s.DefaultChannelID) {
		if ch != 0 && slices.Contains(s.IgnoreChannelIDs, ch) {
			return invalid("log channel %s is also on the ignore list", ch)
		}
	}
	return nil
}

func uniqueIDs(field string, in []snowflake.ID, max int) ([]snowflake.ID, error) {
	out := []snowflake.ID{}
	for _, id := range in {
		if id != 0 && !slices.Contains(out, id) {
			out = append(out, id)
		}
	}
	if len(out) > max {
		return nil, invalid("%s: at most %d", field, max)
	}
	return out, nil
}

// Destination is the channel an entry of kind k goes to, or 0 if the kind
// is off or no channel is set for it.
func (s Settings) Destination(k Kind) snowflake.ID {
	if !s.Events[k] {
		return 0
	}
	if ch := s.Channels.For(k.Category()); ch != 0 {
		return ch
	}
	return s.DefaultChannelID
}

// ReferencedChannels are every channel the settings mention (for checks
// that they exist).
func (s Settings) ReferencedChannels() []snowflake.ID {
	out := s.Channels.All()
	if s.DefaultChannelID != 0 && !slices.Contains(out, s.DefaultChannelID) {
		out = append(out, s.DefaultChannelID)
	}
	return append(out, s.IgnoreChannelIDs...)
}

// cachesMessages reports whether messages must be cached: only when edits
// or deletes are actually logged somewhere.
func (s Settings) cachesMessages() bool {
	return s.Destination(KindMessageEdit) != 0 || s.Destination(KindMessageDelete) != 0 ||
		s.Destination(KindMessageBulkDelete) != 0
}

// ignored reports whether message or voice activity is ignored.
func (s Settings) ignored(channelID, parentID snowflake.ID, roleIDs []snowflake.ID, bot bool) bool {
	if bot && s.IgnoreBots {
		return true
	}
	if slices.Contains(s.IgnoreChannelIDs, channelID) || (parentID != 0 && slices.Contains(s.IgnoreChannelIDs, parentID)) {
		return true
	}
	for _, r := range roleIDs {
		if slices.Contains(s.IgnoreRoleIDs, r) {
			return true
		}
	}
	return false
}

// LoadSettings reads a server's logging settings.
func LoadSettings(ctx context.Context, cfg *core.ConfigService, guildID snowflake.ID) (Settings, error) {
	raw, err := cfg.Settings(ctx, guildID, core.ModuleLogging)
	if err != nil {
		return Settings{}, err
	}
	var s Settings
	return s, json.Unmarshal(raw, &s)
}
