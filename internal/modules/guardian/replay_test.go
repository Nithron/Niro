package guardian

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"testing"
	"time"

	"github.com/disgoorg/snowflake/v2"
)

func discardLogger() *slog.Logger { return slog.New(slog.NewTextHandler(io.Discard, nil)) }

// The replay harness feeds recorded message and join sequences through the
// real Service (with fake platform, store and clock) and checks what
// Guardian did. Scenarios live in testdata/*.json; add one for every false
// positive or miss found in the wild.
type scenario struct {
	Description  string                    `json:"description"`
	Settings     json.RawMessage           `json:"settings"`
	Blocklist    []string                  `json:"blocklist"`
	WatchingDays int                       `json:"watching_days"`
	Members      map[string]scenarioMember `json:"members"`
	Steps        []scenarioStep            `json:"steps"`
	Expect       scenarioExpect            `json:"expect"`
}

type scenarioMember struct {
	AccountDays  int  `json:"account_days"`
	JoinedDays   int  `json:"joined_days"`
	RolePosition int  `json:"role_position"`
	SilentDays   int  `json:"silent_days"` // days since last message; 0 = active today
	Moderator    bool `json:"moderator"`
	Bot          bool `json:"bot"`
	Joins        bool `json:"joins"` // not a member yet; a join step adds them
}

type scenarioStep struct {
	AfterMS int `json:"after_ms"`
	Message *struct {
		User        string   `json:"user"`
		Channel     uint64   `json:"channel"`
		Content     string   `json:"content"`
		Attachments []string `json:"attachments"`
	} `json:"message"`
	Join *struct {
		User string `json:"user"`
	} `json:"join"`
}

type scenarioExpect struct {
	Responses    map[string]Response `json:"responses"` // step index → response; others must be none
	DeletedSteps []int               `json:"deleted_steps"`
	TimedOut     []string            `json:"timed_out"`
	Kicked       []string            `json:"kicked"`
	Events       []struct {
		Kind     Kind     `json:"kind"`
		Response Response `json:"response"`
		Status   Status   `json:"status"`
		User     string   `json:"user"`
		Monitor  bool     `json:"monitor"`
	} `json:"events"`
	RaidActive    bool `json:"raid_active"`
	InvitesPaused bool `json:"invites_paused"`
}

func TestReplayScenarios(t *testing.T) {
	files, err := filepath.Glob("testdata/*.json")
	if err != nil || len(files) == 0 {
		t.Fatalf("no scenarios found: %v", err)
	}
	for _, f := range files {
		t.Run(filepath.Base(f), func(t *testing.T) { runScenario(t, f) })
	}
}

func runScenario(t *testing.T, path string) {
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var sc scenario
	if err := json.Unmarshal(raw, &sc); err != nil {
		t.Fatalf("parsing scenario: %v", err)
	}
	settingsJSON, err := SettingsSpec{}.Normalize(sc.Settings)
	if err != nil {
		t.Fatalf("scenario settings: %v", err)
	}
	var settings Settings
	_ = json.Unmarshal(settingsJSON, &settings)

	h := newHarness(settings)
	h.svc.d.Blocklist.Replace(sc.Blocklist)
	h.tracker.SetWatchingSince(testGuild, h.now.Add(-time.Duration(sc.WatchingDays)*24*time.Hour))
	start := h.now

	ids := map[string]snowflake.ID{}
	names := map[snowflake.ID]string{}
	meta := map[string]scenarioMember{}
	for name, m := range sc.Members {
		if m.AccountDays == 0 && !m.Joins {
			m.AccountDays = 365
		}
		meta[name] = m
		id := h.addMember(name, m.AccountDays, m.RolePosition)
		if m.Joins {
			delete(h.platform.members, id)
		} else {
			h.tracker.SetActive(testGuild, id, h.now.Add(-time.Duration(m.SilentDays)*24*time.Hour-time.Hour))
		}
		ids[name], names[id] = id, name
	}
	idFor := func(name string) snowflake.ID {
		if id, ok := ids[name]; ok {
			return id
		}
		t.Fatalf("unknown member %q", name)
		return 0
	}

	ctx := context.Background()
	stepMsg := map[int]snowflake.ID{}
	responses := map[int]Response{}
	for i, st := range sc.Steps {
		h.now = start.Add(time.Duration(st.AfterMS) * time.Millisecond)
		switch {
		case st.Message != nil:
			id := idFor(st.Message.User)
			m := meta[st.Message.User]
			msgID := snowflake.New(h.now) + snowflake.ID(i)
			stepMsg[i] = msgID
			v, err := h.svc.HandleMessage(ctx, Message{
				GuildID: testGuild, ChannelID: snowflake.ID(st.Message.Channel), ID: msgID, AuthorID: id,
				AuthorName: st.Message.User, AuthorBot: m.Bot, AuthorModerator: m.Moderator,
				AuthorJoinedAt: h.now.Add(-time.Duration(m.JoinedDays) * 24 * time.Hour),
				Content:        st.Message.Content, Attachments: st.Message.Attachments, CreatedAt: h.now,
			})
			if err != nil {
				t.Fatalf("step %d: %v", i, err)
			}
			responses[i] = v.Response
		case st.Join != nil:
			id := idFor(st.Join.User)
			m := meta[st.Join.User]
			h.platform.members[id] = &Member{Name: st.Join.User}
			h.platform.members[id].ID = id
			if err := h.svc.HandleJoin(ctx, Join{GuildID: testGuild, UserID: id, UserName: st.Join.User, Bot: m.Bot, At: h.now}); err != nil {
				t.Fatalf("step %d: %v", i, err)
			}
		default:
			t.Fatalf("step %d has no message or join", i)
		}
	}

	ex := sc.Expect
	for i, st := range sc.Steps {
		if st.Message == nil {
			continue
		}
		want := ex.Responses[strconv.Itoa(i)]
		if responses[i] != want {
			t.Errorf("step %d (%s): response %q, want %q", i, st.Message.Content, responses[i], want)
		}
	}

	var deleted []int
	for i, msgID := range stepMsg {
		if slices.ContainsFunc(h.platform.deleted, func(r MessageRef) bool { return r.MessageID == msgID }) {
			deleted = append(deleted, i)
		}
	}
	slices.Sort(deleted)
	if !slices.Equal(deleted, sortedInts(ex.DeletedSteps)) {
		t.Errorf("deleted steps %v, want %v", deleted, ex.DeletedSteps)
	}

	var timedOut []string
	for id := range h.platform.timeouts {
		timedOut = append(timedOut, names[id])
	}
	if !slices.Equal(sortedStrings(timedOut), sortedStrings(ex.TimedOut)) {
		t.Errorf("timed out %v, want %v", timedOut, ex.TimedOut)
	}
	var kicked []string
	for _, id := range h.platform.kicked {
		kicked = append(kicked, names[id])
	}
	if !slices.Equal(sortedStrings(kicked), sortedStrings(ex.Kicked)) {
		t.Errorf("kicked %v, want %v", kicked, ex.Kicked)
	}

	if len(h.store.events) != len(ex.Events) {
		t.Errorf("%d events, want %d: %+v", len(h.store.events), len(ex.Events), h.store.events)
	} else {
		for i, want := range ex.Events {
			got := h.store.events[i]
			if got.Kind != want.Kind || got.Response != want.Response || got.Status != want.Status ||
				names[got.UserID] != want.User || got.Evidence.Monitor != want.Monitor {
				t.Errorf("event %d = {%s %s %s %s monitor=%v}, want %+v", i, got.Kind, got.Response, got.Status,
					names[got.UserID], got.Evidence.Monitor, want)
			}
		}
	}

	_, err = h.store.ActiveRaid(ctx, testGuild)
	if active := err == nil; active != ex.RaidActive {
		t.Errorf("raid active = %v, want %v", active, ex.RaidActive)
	}
	if paused := !h.platform.invitesUntil.IsZero(); paused != ex.InvitesPaused {
		t.Errorf("invites paused = %v, want %v", paused, ex.InvitesPaused)
	}
}

func sortedInts(s []int) []int {
	s = slices.Clone(s)
	slices.Sort(s)
	if s == nil {
		return []int{}
	}
	return s
}

func sortedStrings(s []string) []string {
	s = slices.Clone(s)
	slices.Sort(s)
	if s == nil {
		return []string{}
	}
	return s
}
