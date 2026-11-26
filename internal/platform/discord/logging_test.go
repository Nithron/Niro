package discord

import (
	"encoding/json"
	"io"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/disgoorg/disgo/discord"
	"github.com/disgoorg/disgo/gateway"
	"github.com/disgoorg/snowflake/v2"

	"github.com/nithron/niro/internal/i18n"
	"github.com/nithron/niro/internal/modules/logging"
)

func TestLogCardForEveryKind(t *testing.T) {
	p := testPresenter(t)
	at := time.Date(2026, 11, 26, 12, 0, 0, 0, time.UTC)
	for _, k := range logging.Kinds {
		e := logging.Entry{Kind: k, GuildID: 1, At: at, UserID: 100, UserName: "alice", ChannelID: 10, OldChannelID: 11,
			TargetID: 12, TargetName: "general", ActorID: 200, ActorName: "mod", Reason: "because",
			Changes: []logging.Change{{Key: "name", Old: "old", New: "new"}}, Count: 2, Action: "quarantine", RuleName: "scam"}
		if k == logging.KindAutoModAction {
			e.Action, e.RuleName, e.Keyword, e.Before = "block_message", "Invites", "discord.gg/*", "join discord.gg/x"
		}
		card, file := p.logCard("en", e, "Test server", "general")
		text, _ := cardParts(card)
		title := p.t("en", i18n.LogTitle(string(k)), nil)
		if title == string(i18n.LogTitle(string(k))) || !strings.Contains(text, title) {
			t.Errorf("%s: no title in\n%s", k, text)
		}
		if strings.Contains(text, "log.") || strings.Contains(text, "<no value>") {
			t.Errorf("%s: untranslated text:\n%s", k, text)
		}
		if (file != nil) != (k == logging.KindMessageBulkDelete) {
			t.Errorf("%s: file = %v", k, file != nil)
		}
		if n := len([]rune(text)); n > 4000 {
			t.Errorf("%s: %d characters, over Discord's limit", k, n)
		}
	}
}

func TestLogCardShowsTextSafely(t *testing.T) {
	p := testPresenter(t)
	long := strings.Repeat("a", 5000)
	e := logging.Entry{Kind: logging.KindMessageEdit, GuildID: 1, ChannelID: 2, MessageID: 3, UserID: 4, Cached: true,
		Before: "hi @everyone ````break out``` and ``````", After: long, At: time.Now()}
	text, _ := cardParts(func() discord.ContainerComponent { c, _ := p.logCard("en", e, "", ""); return c }())
	if strings.Count(text, "```") != 4 {
		t.Errorf("user text escaped its code block:\n%s", text)
	}
	if !strings.Contains(text, "https://discord.com/channels/1/2/3") || len([]rune(text)) > 4000 {
		t.Errorf("edit card wrong (len %d)", len([]rune(text)))
	}
	e.ContentHidden = true
	text, _ = cardParts(func() discord.ContainerComponent { c, _ := p.logCard("en", e, "", ""); return c }())
	if strings.Contains(text, "everyone") || !strings.Contains(text, "content logging is off") {
		t.Errorf("hidden content shown:\n%s", text)
	}
}

func TestBulkDeleteFile(t *testing.T) {
	p := testPresenter(t)
	at := time.Date(2026, 11, 26, 12, 0, 0, 0, time.UTC)
	e := logging.Entry{Kind: logging.KindMessageBulkDelete, GuildID: 1, ChannelID: 10, At: at, Count: 3, ByNiro: true, ByNiroSource: "purge",
		Messages: []logging.CachedMessage{{ID: 5, AuthorID: 7, AuthorName: "bob", Content: "spam", CreatedAt: at}}}
	card, file := p.logCard("en", e, "Test server", "general")
	text, _ := cardParts(card)
	if !strings.Contains(text, "/purge") || !strings.Contains(text, "**Messages:** 3") {
		t.Errorf("card:\n%s", text)
	}
	body, _ := io.ReadAll(file.Reader)
	got := string(body)
	for _, want := range []string{"3 messages deleted in #general (Test server)", "Deleted by Niro (/purge)", "bob (7): spam", "+2 (not in the cache"} {
		if !strings.Contains(got, want) {
			t.Errorf("file missing %q:\n%s", want, got)
		}
	}
	if !strings.HasSuffix(file.Name, ".txt") {
		t.Errorf("file name %q", file.Name)
	}
}

func TestFormatChange(t *testing.T) {
	raw := func(v any) json.RawMessage { b, _ := json.Marshal(v); return b }
	cases := []struct {
		key      string
		old, new any
		want     logging.Change
		ok       bool
	}{
		{"name", "old", "new", logging.Change{Key: "name", Old: "old", New: "new"}, true},
		{"nsfw", false, true, logging.Change{Key: "nsfw", Old: "✗", New: "✓"}, true},
		{"color", 0, 0xFF0000, logging.Change{Key: "color", Old: "#000000", New: "#FF0000"}, true},
		{"verification_level", 0, 3, logging.Change{Key: "verification_level", Old: "None", New: "High"}, true},
		{"parent_id", nil, "55", logging.Change{Key: "parent_id", New: "<#55>"}, true},
		{"permissions", "0", "8", logging.Change{Key: "permissions", Old: "None", New: "Administrator"}, true},
		{"id", "1", "2", logging.Change{}, false},
		{"$add", nil, []map[string]string{{"id": "50", "name": "Staff"}}, logging.Change{Key: "$add", New: "<@&50>"}, true},
	}
	for _, c := range cases {
		ch := discord.AuditLogChange{Key: discord.AuditLogChangeKey(c.key)}
		if c.old != nil {
			ch.OldValue = raw(c.old)
		}
		if c.new != nil {
			ch.NewValue = raw(c.new)
		}
		got, ok := formatChange(ch)
		if ok != c.ok || got != c.want {
			t.Errorf("%s: got %+v (%v), want %+v (%v)", c.key, got, ok, c.want, c.ok)
		}
	}
}

func TestSendQueueKeepsOrderAndBounds(t *testing.T) {
	var mu sync.Mutex
	var got []string
	release := make(chan struct{})
	q := newSendQueue(func(_ snowflake.ID, m queuedLog) {
		<-release
		mu.Lock()
		got = append(got, m.msg.Content)
		mu.Unlock()
	})
	q.size, q.idle = 3, 50*time.Millisecond
	accepted := 0
	for i := range 10 {
		if q.enqueue(1, queuedLog{msg: discord.MessageCreate{Content: string(rune('a' + i))}}) {
			accepted++
		}
	}
	// One message is with the sender, three wait in the queue; the rest are dropped.
	if accepted < 3 || accepted > 4 {
		t.Errorf("accepted %d of 10 with a queue of 3", accepted)
	}
	close(release)
	deadline := time.Now().Add(2 * time.Second)
	for {
		mu.Lock()
		n := len(got)
		mu.Unlock()
		if n == accepted || time.Now().After(deadline) {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	mu.Lock()
	defer mu.Unlock()
	for i := 1; i < len(got); i++ {
		if got[i] < got[i-1] {
			t.Errorf("out of order: %v", got)
		}
	}
	// The idle worker exits and a new one starts on demand.
	time.Sleep(150 * time.Millisecond)
	q.mu.Lock()
	idle := len(q.queues)
	q.mu.Unlock()
	if idle != 0 {
		t.Errorf("%d idle workers left", idle)
	}
}

func TestBulkDeleteHandlerIsReplaced(t *testing.T) {
	b := &Bot{}
	hs := b.gatewayHandlers()
	if _, ok := hs[gateway.EventTypeMessageCreate]; !ok {
		t.Fatal("default handlers missing")
	}
	h, ok := hs[gateway.EventTypeMessageDeleteBulk]
	if !ok || h.EventType() != gateway.EventTypeMessageDeleteBulk {
		t.Fatal("bulk delete handler missing")
	}
}

func TestIntentsForLogging(t *testing.T) {
	for _, i := range []gateway.Intents{gateway.IntentGuildModeration, gateway.IntentGuildVoiceStates,
		gateway.IntentGuildInvites, gateway.IntentAutoModerationExecution} {
		if !Intents.Has(i) {
			t.Errorf("missing intent %d", i)
		}
	}
	if Intents.Has(gateway.IntentGuildPresences) {
		t.Error("the presence intent must never be requested")
	}
}

func TestChangeValuesAreEscapedAndBounded(t *testing.T) {
	p := testPresenter(t)
	long := "<" + strings.Repeat("x", 3000) + ">"
	var changes []logging.Change
	for range 30 {
		changes = append(changes, logging.Change{Key: "topic", Old: long, New: "<https://evil.example|click>"})
	}
	e := logging.Entry{Kind: logging.KindChannelUpdate, GuildID: 1, TargetID: 2, At: time.Now(), Changes: changes}
	card, _ := p.logCard("en", e, "", "")
	text, _ := cardParts(card)
	if len([]rune(text)) > 4000 {
		t.Errorf("card has %d characters", len([]rune(text)))
	}
	if strings.Contains(text, "<https://evil.example|click>") {
		t.Errorf("unescaped link-like value:\n%s", text)
	}
	if !isMention("<#123>") || !isMention("<@&5>") || isMention("<#12 3>") || isMention(long) {
		t.Error("isMention is too loose or too strict")
	}
}
