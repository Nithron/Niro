package logging

import (
	"encoding/json"
	"errors"
	"testing"

	"github.com/disgoorg/snowflake/v2"
)

func normalize(t *testing.T, raw string) (Settings, error) {
	t.Helper()
	var in []byte
	if raw != "" {
		in = []byte(raw)
	}
	out, err := SettingsSpec{}.Normalize(in)
	if err != nil {
		return Settings{}, err
	}
	var s Settings
	if err := json.Unmarshal(out, &s); err != nil {
		t.Fatal(err)
	}
	return s, nil
}

func TestDefaultsLogNothingUntilAChannelIsSet(t *testing.T) {
	s, err := normalize(t, "")
	if err != nil {
		t.Fatal(err)
	}
	for _, k := range Kinds {
		if d := s.Destination(k); d != 0 {
			t.Errorf("%s goes to %s with default settings", k, d)
		}
	}
	if !s.IgnoreBots || s.NoMessageContent {
		t.Errorf("unexpected defaults: %+v", s)
	}
	if len(s.Events) != len(Kinds) {
		t.Errorf("events has %d kinds, want %d", len(s.Events), len(Kinds))
	}
	if s.Events[KindVoiceJoin] || !s.Events[KindMessageDelete] {
		t.Errorf("voice should default off and messages on: %v", s.Events)
	}
}

func TestDestinationRouting(t *testing.T) {
	s, err := normalize(t, `{"default_channel_id":"100","channels":{"moderation":"200"},"events":{"member_join":false,"voice_join":true}}`)
	if err != nil {
		t.Fatal(err)
	}
	cases := map[Kind]snowflake.ID{
		KindMessageDelete: 100, // falls back to the default channel
		KindMemberBan:     200, // its category's channel
		KindMemberJoin:    0,   // turned off
		KindVoiceJoin:     100, // turned on
		KindVoiceLeave:    0,   // still off by default
	}
	for k, want := range cases {
		if got := s.Destination(k); got != want {
			t.Errorf("Destination(%s) = %s, want %s", k, got, want)
		}
	}
}

func TestMissingKindsGetTheirDefault(t *testing.T) {
	// A document saved before a kind existed only lists older kinds.
	s, err := normalize(t, `{"events":{"message_edit":false}}`)
	if err != nil {
		t.Fatal(err)
	}
	if s.Events[KindMessageEdit] || !s.Events[KindGuardianAction] || s.Events[KindVoiceMove] {
		t.Errorf("events = %v", s.Events)
	}
}

func TestSettingsValidation(t *testing.T) {
	many := func(n int) string {
		out := "["
		for i := 1; i <= n; i++ {
			if i > 1 {
				out += ","
			}
			out += `"` + snowflake.ID(i).String() + `"`
		}
		return out + "]"
	}
	bad := map[string]string{
		"unknown field":       `{"colour":"red"}`,
		"unknown event":       `{"events":{"message_explode":true}}`,
		"too many channels":   `{"ignore_channel_ids":` + many(MaxIgnoreChannels+1) + `}`,
		"too many roles":      `{"ignore_role_ids":` + many(MaxIgnoreRoles+1) + `}`,
		"log channel ignored": `{"channels":{"messages":"5"},"ignore_channel_ids":["5"]}`,
		"default ignored":     `{"default_channel_id":"5","ignore_channel_ids":["5"]}`,
	}
	for name, raw := range bad {
		if _, err := normalize(t, raw); !errors.Is(err, ErrInvalidSettings) {
			t.Errorf("%s: err = %v, want ErrInvalidSettings", name, err)
		}
	}
	s, err := normalize(t, `{"ignore_channel_ids":["1","1","0","2"]}`)
	if err != nil {
		t.Fatal(err)
	}
	if len(s.IgnoreChannelIDs) != 2 {
		t.Errorf("ignore list not deduplicated: %v", s.IgnoreChannelIDs)
	}
}

func TestIgnored(t *testing.T) {
	s, _ := normalize(t, `{"ignore_channel_ids":["10"],"ignore_role_ids":["20"]}`)
	cases := []struct {
		name            string
		channel, parent snowflake.ID
		roles           []snowflake.ID
		bot             bool
		want            bool
	}{
		{"plain", 1, 0, nil, false, false},
		{"ignored channel", 10, 0, nil, false, true},
		{"thread in ignored channel", 11, 10, nil, false, true},
		{"ignored role", 1, 0, []snowflake.ID{5, 20}, false, true},
		{"bot", 1, 0, nil, true, true},
	}
	for _, c := range cases {
		if got := s.ignored(c.channel, c.parent, c.roles, c.bot); got != c.want {
			t.Errorf("%s: ignored = %v, want %v", c.name, got, c.want)
		}
	}
	s.IgnoreBots = false
	if s.ignored(1, 0, nil, true) {
		t.Error("bots are ignored with ignore_bots off")
	}
}
