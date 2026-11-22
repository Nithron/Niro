package discord

import (
	"encoding/json"
	"io"
	"log/slog"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/disgoorg/disgo/discord"
	"github.com/disgoorg/snowflake/v2"

	"github.com/nithron/niro/internal/i18n"
	"github.com/nithron/niro/internal/modules/guardian"
)

func testPresenter(t *testing.T) presenter {
	t.Helper()
	cat, err := i18n.Load()
	if err != nil {
		t.Fatal(err)
	}
	return presenter{cat: cat, emojis: NewEmojis(&fakeApps{}, 1, slog.New(slog.NewTextHandler(io.Discard, nil)))}
}

// cardParts flattens a card into its text and its button custom IDs.
func cardParts(c discord.ContainerComponent) (string, []string) {
	var text []string
	var buttons []string
	for _, sub := range c.Components {
		switch v := sub.(type) {
		case discord.TextDisplayComponent:
			text = append(text, v.Content)
		case discord.ActionRowComponent:
			for _, b := range v.Components {
				if btn, ok := b.(discord.ButtonComponent); ok {
					buttons = append(buttons, btn.CustomID)
				}
			}
		}
	}
	return strings.Join(text, "\n"), buttons
}

func TestGuardianCardButtonsFollowStatus(t *testing.T) {
	p := testPresenter(t)
	ev := guardian.Event{
		ID: 7, GuildID: 1, UserID: 100, UserName: "victim*", Kind: guardian.KindCompromised, Response: guardian.ResponseQuarantine,
		Signals:  []guardian.Signal{{Kind: guardian.SignalBurst, Detail: "l", Weight: 3}, {Kind: guardian.SignalBlockedDomain, Detail: "evil.ru", Weight: 3}},
		Evidence: guardian.Evidence{Channels: []snowflake.ID{11, 12, 13}, Deleted: 3, Domains: []string{"evil.ru"}},
		Status:   guardian.StatusOpen, CaseNumber: 4, CreatedAt: time.Now(),
	}
	text, buttons := cardParts(p.guardianCard("en", ev))
	for _, want := range []string{"Possible hacked account", "victim\\*", "<#11> <#12> <#13>", "`evil.ru`", "Messages deleted:** 3", "case #4", "Guardian event #7"} {
		if !strings.Contains(text, want) {
			t.Errorf("card text missing %q:\n%s", want, text)
		}
	}
	if !slices.Equal(buttons, []string{"/guardian/resolve/7/restore", "/guardian/resolve/7/ban", "/guardian/resolve/7/keep"}) {
		t.Errorf("open buttons = %v", buttons)
	}

	ev.Status, ev.ResolvedByName = guardian.StatusKept, "Mod"
	text, buttons = cardParts(p.guardianCard("en", ev))
	if len(buttons) != 2 || !strings.Contains(text, "**Mod** kept the quarantine") {
		t.Errorf("kept: buttons %v text %s", buttons, text)
	}
	ev.Status = guardian.StatusRestored
	if _, buttons = cardParts(p.guardianCard("en", ev)); len(buttons) != 0 {
		t.Errorf("restored: buttons %v", buttons)
	}

	raid := guardian.Event{ID: 9, Kind: guardian.KindRaid, Response: guardian.ResponseRaidMode, Status: guardian.StatusActive,
		Evidence: guardian.Evidence{Joins: 12, Problems: []string{"pause_invites_failed"}}}
	text, buttons = cardParts(p.guardianCard("en", raid))
	if !slices.Equal(buttons, []string{"/guardian/raid/end"}) || !strings.Contains(text, "Pausing invites failed") {
		t.Errorf("raid card: %v\n%s", buttons, text)
	}

	monitor := guardian.Event{ID: 10, UserID: 1, Kind: guardian.KindScam, Response: guardian.ResponseQuarantine, Status: guardian.StatusInfo,
		Evidence: guardian.Evidence{Monitor: true}}
	text, buttons = cardParts(p.guardianCard("en", monitor))
	if len(buttons) != 0 || !strings.Contains(text, "Monitor mode") || strings.Contains(text, "**Action:**") {
		t.Errorf("monitor card: %v\n%s", buttons, text)
	}
}

func TestAuditCard(t *testing.T) {
	p := testPresenter(t)
	findings := guardian.Audit(guardian.AuditInput{
		EveryonePermissions: []string{guardian.PermAdministrator},
		Roles:               []guardian.AuditRole{{ID: 1, Name: "Mods", Permissions: []string{guardian.PermBanMembers}, Members: -1}},
	})
	text, _ := cardParts(p.auditCard("en", "Test", findings, false))
	for _, want := range []string{"Critical", "**@everyone** has **Administrator**", "Role **Mods** has **Ban Members** (? members)", "Manage Webhooks permission"} {
		if !strings.Contains(text, want) {
			t.Errorf("audit text missing %q:\n%s", want, text)
		}
	}
	clean, _ := cardParts(p.auditCard("en", "Test", nil, true))
	if !strings.Contains(clean, "No problems found") {
		t.Errorf("clean audit: %s", clean)
	}
}

func TestDangerousNames(t *testing.T) {
	got := dangerousNames(discord.PermissionAdministrator | discord.PermissionSendMessages | discord.PermissionMentionEveryone)
	if !slices.Equal(got, []string{guardian.PermAdministrator, guardian.PermMentionEveryone}) {
		t.Errorf("names = %v", got)
	}
	// Every audit permission has a translation.
	cat, _ := i18n.Load()
	for _, a := range auditPermissions {
		if k := i18n.GuardianPerm(a.name); cat.T("en", k, nil) == string(k) {
			t.Errorf("missing %s", k)
		}
	}
}

func TestGuardianCommandPermissions(t *testing.T) {
	want := map[string]discord.Permissions{
		"guardian": discord.PermissionManageGuild,
		"lockdown": discord.PermissionManageChannels,
		"unlock":   discord.PermissionManageChannels,
	}
	for _, c := range testBot(t).commands() {
		s := c.(discord.SlashCommandCreate)
		perm, ok := want[s.Name]
		if !ok {
			continue
		}
		delete(want, s.Name)
		if p := s.DefaultMemberPermissions.Or(nil); p == nil || *p != perm {
			t.Errorf("/%s default permissions = %v", s.Name, p)
		}
		raw, _ := json.Marshal(s)
		if s.Name == "guardian" && !strings.Contains(string(raw), `"name":"raid"`) {
			t.Error("/guardian raid group missing")
		}
	}
	if len(want) > 0 {
		t.Errorf("missing commands: %v", want)
	}
}

func TestLockBitsKeepReading(t *testing.T) {
	if lockBits.Has(discord.PermissionViewChannel) || lockBits.Has(discord.PermissionReadMessageHistory) {
		t.Error("a lockdown must not hide the channel or its history")
	}
	if !lockBits.Has(discord.PermissionSendMessages) || !lockBits.Has(discord.PermissionSendMessagesInThreads) {
		t.Error("a lockdown must stop sending, including in threads")
	}
}
