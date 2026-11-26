package discord

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/disgoorg/disgo/discord"
	"github.com/disgoorg/snowflake/v2"

	"github.com/nithron/niro/internal/core/message"
)

func TestWelcomeMentionsOnlyTheMember(t *testing.T) {
	d := message.Simple("Hi <@100> @everyone <@&5>")
	m := documentMessage(renderDocument(d), &discord.AllowedMentions{Users: []snowflake.ID{100}})
	raw, _ := json.Marshal(m)
	s := string(raw)
	if !strings.Contains(s, `"allowed_mentions":{"parse":null,"roles":null,"users":["100"]`) {
		t.Errorf("allowed mentions: %s", s)
	}
}
