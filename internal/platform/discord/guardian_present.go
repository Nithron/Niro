package discord

import (
	"context"
	"fmt"
	"strconv"
	"strings"

	"github.com/disgoorg/disgo/bot"
	"github.com/disgoorg/disgo/discord"
	"github.com/disgoorg/disgo/rest"
	"github.com/disgoorg/snowflake/v2"

	"github.com/nithron/niro/internal/i18n"
	"github.com/nithron/niro/internal/modules/guardian"
)

// guardianAlerts implements guardian.Alerter on Discord.
type guardianAlerts struct {
	client *bot.Client
	p      presenter
	locale func(snowflake.ID) string
}

var _ guardian.Alerter = guardianAlerts{}

func (a guardianAlerts) RecoveryDM(ctx context.Context, ev guardian.Event, guildName string) error {
	ch, err := a.client.Rest.CreateDMChannel(ev.UserID, rest.WithCtx(ctx))
	if err != nil {
		return err
	}
	l := a.locale(ev.GuildID)
	card := discord.NewContainer(discord.NewTextDisplay(
		"### " + strings.TrimSpace(a.p.emojis.Get(EmojiShield)+" "+a.p.t(l, i18n.GuardianDmTitle, nil)) + "\n" +
			a.p.t(l, i18n.GuardianDmBody, map[string]any{"Server": escapeMarkdown(guildName)}),
	)).WithAccentColor(colorWarning)
	_, err = a.client.Rest.CreateMessage(ch.ID(), discord.MessageCreate{
		Flags: discord.MessageFlagIsComponentsV2, Components: []discord.LayoutComponent{card}, AllowedMentions: noPings,
	}, rest.WithCtx(ctx))
	return err
}

func (a guardianAlerts) PostAlert(ctx context.Context, channelID snowflake.ID, ev guardian.Event) (snowflake.ID, error) {
	m, err := a.client.Rest.CreateMessage(channelID, discord.MessageCreate{
		Flags:           discord.MessageFlagIsComponentsV2,
		Components:      []discord.LayoutComponent{a.p.guardianCard(a.locale(ev.GuildID), ev)},
		AllowedMentions: noPings,
	}, rest.WithCtx(ctx))
	if err != nil {
		return 0, err
	}
	return m.ID, nil
}

func (a guardianAlerts) EditAlert(ctx context.Context, channelID, messageID snowflake.ID, ev guardian.Event) error {
	comps := []discord.LayoutComponent{a.p.guardianCard(a.locale(ev.GuildID), ev)}
	_, err := a.client.Rest.UpdateMessage(channelID, messageID, discord.MessageUpdate{Components: &comps, AllowedMentions: noPings}, rest.WithCtx(ctx))
	return err
}

func guardianColor(ev guardian.Event) int {
	switch ev.Status {
	case guardian.StatusOpen, guardian.StatusActive, guardian.StatusBanned:
		return colorDanger
	case guardian.StatusKept:
		return colorOrange
	case guardian.StatusRestored, guardian.StatusEnded:
		return colorSuccess
	}
	if ev.Response == guardian.ResponseDelete || ev.Response == guardian.ResponseKick {
		return colorOrange
	}
	return colorWarning
}

// signalList renders signals as "Label (`detail`) · Label".
func (p presenter) signalList(l string, signals []guardian.Signal) string {
	parts := make([]string, 0, len(signals))
	for _, s := range signals {
		part := p.t(l, i18n.GuardianSignal(string(s.Kind)), nil)
		if s.Detail != "" && s.Kind != guardian.SignalManual {
			part += " (`" + strings.ReplaceAll(s.Detail, "`", "'") + "`)"
		}
		parts = append(parts, part)
	}
	return strings.Join(parts, " · ")
}

func channelMentions(ids []snowflake.ID, limit int) string {
	parts := make([]string, 0, len(ids))
	for i, id := range ids {
		if i == limit {
			parts = append(parts, fmt.Sprintf("+%d", len(ids)-limit))
			break
		}
		parts = append(parts, "<#"+id.String()+">")
	}
	return strings.Join(parts, " ")
}

// guardianCard renders a Guardian event for the alert channel.
func (p presenter) guardianCard(l string, ev guardian.Event) discord.ContainerComponent {
	emoji := p.emojis.Get(EmojiShield)
	if ev.Kind == guardian.KindRaid {
		emoji = p.emojis.Get(EmojiWarn)
	}
	lines := []string{"### " + strings.TrimSpace(emoji+" "+p.t(l, i18n.GuardianTitle(string(ev.Kind)), nil))}
	add := func(k i18n.Key, data map[string]any) { lines = append(lines, p.t(l, k, data)) }

	if ev.UserID != 0 {
		add(i18n.GuardianAlertUser, map[string]any{"User": userRef(ev.UserID, ev.UserName)})
	}
	if len(ev.Signals) > 0 {
		add(i18n.GuardianAlertWhy, map[string]any{"Signals": p.signalList(l, ev.Signals)})
	}
	e := ev.Evidence
	if len(e.Channels) > 0 && ev.Kind != guardian.KindRaid && ev.Kind != guardian.KindAccountAge {
		add(i18n.GuardianAlertChannels, map[string]any{"Channels": channelMentions(e.Channels, 10)})
	}
	if e.Deleted > 0 {
		add(i18n.GuardianAlertDeleted, map[string]any{"Count": e.Deleted})
	}
	if len(e.Domains) > 0 {
		ds := make([]string, len(e.Domains))
		for i, d := range e.Domains {
			ds[i] = "`" + d + "`"
		}
		add(i18n.GuardianAlertDomains, map[string]any{"Domains": strings.Join(ds, ", ")})
	}
	if e.Joins > 0 {
		add(i18n.GuardianAlertJoins, map[string]any{"Count": e.Joins})
	}
	if e.AccountAge != "" {
		add(i18n.GuardianAlertAccountAge, map[string]any{"Age": e.AccountAge})
	}
	if !e.Monitor {
		action := p.t(l, i18n.GuardianAction(string(ev.Response)), nil)
		if ev.CaseNumber > 0 {
			action += " · " + p.t(l, i18n.GuardianAlertCase, map[string]any{"Number": ev.CaseNumber})
		}
		add(i18n.GuardianAlertAction, map[string]any{"Action": action})
	}
	if len(e.Problems) > 0 {
		lines = append(lines, p.t(l, i18n.GuardianAlertProblems, nil))
		for _, pr := range e.Problems {
			lines = append(lines, "- "+p.t(l, i18n.GuardianProblem(pr), nil))
		}
	}
	if e.Monitor {
		lines = append(lines, "-# "+p.t(l, i18n.GuardianAlertMonitor, nil))
	}

	comps := []discord.ContainerSubComponent{discord.NewTextDisplay(strings.Join(lines, "\n"))}
	if ev.Status != guardian.StatusInfo {
		by := "**" + escapeMarkdown(ev.ResolvedByName) + "**"
		if ev.ResolvedByName == "" {
			by = "Guardian"
		}
		status := p.t(l, i18n.GuardianStatus(string(ev.Status)), map[string]any{"By": by})
		comps = append(comps, discord.NewSmallSeparator(), discord.NewTextDisplay(status))
	}
	if row, ok := p.guardianButtons(l, ev); ok {
		comps = append(comps, row)
	}
	comps = append(comps, discord.NewTextDisplay("-# "+p.t(l, i18n.GuardianAlertEvent, map[string]any{"ID": ev.ID})+" · "+ts(ev.CreatedAt, "f")))
	return discord.NewContainer(comps...).WithAccentColor(guardianColor(ev))
}

func resolveID(ev guardian.Event, r guardian.Resolution) string {
	return "/guardian/resolve/" + strconv.FormatInt(ev.ID, 10) + "/" + string(r)
}

func (p presenter) guardianButtons(l string, ev guardian.Event) (discord.ActionRowComponent, bool) {
	switch ev.Status {
	case guardian.StatusOpen:
		return discord.NewActionRow(
			discord.NewSuccessButton(p.t(l, i18n.GuardianButtonRestore, nil), resolveID(ev, guardian.ResolveRestore)),
			discord.NewDangerButton(p.t(l, i18n.GuardianButtonBan, nil), resolveID(ev, guardian.ResolveBan)),
			discord.NewSecondaryButton(p.t(l, i18n.GuardianButtonKeep, nil), resolveID(ev, guardian.ResolveKeep)),
		), true
	case guardian.StatusKept:
		return discord.NewActionRow(
			discord.NewSuccessButton(p.t(l, i18n.GuardianButtonRestore, nil), resolveID(ev, guardian.ResolveRestore)),
			discord.NewDangerButton(p.t(l, i18n.GuardianButtonBan, nil), resolveID(ev, guardian.ResolveBan)),
		), true
	case guardian.StatusActive:
		return discord.NewActionRow(
			discord.NewDangerButton(p.t(l, i18n.GuardianButtonEndRaid, nil), "/guardian/raid/end"),
		), true
	}
	return discord.ActionRowComponent{}, false
}

// permissionNames translates permission names into a readable list.
func (p presenter) permissionNames(l string, perms []string) string {
	out := make([]string, len(perms))
	for i, x := range perms {
		out[i] = "**" + p.t(l, i18n.GuardianPerm(x), nil) + "**"
	}
	return strings.Join(out, ", ")
}

// auditText renders one audit finding.
func (p presenter) auditText(l string, f guardian.Finding) string {
	count := strconv.Itoa(f.Count)
	if f.Count < 0 {
		count = p.t(l, i18n.GuardianAuditUnknown, nil)
	}
	return p.t(l, i18n.GuardianAudit(f.Code), map[string]any{
		"Subject": escapeMarkdown(f.Subject), "Permissions": p.permissionNames(l, f.Permissions),
		"Count": count, "Channels": channelMentions(f.ChannelIDs, 5),
	})
}

func severityEmoji(s guardian.Severity) string {
	switch s {
	case guardian.SeverityCritical:
		return "🔴"
	case guardian.SeverityHigh:
		return "🟠"
	case guardian.SeverityMedium:
		return "🟡"
	}
	return "🔵"
}

// auditCard renders audit findings (ephemeral reply).
func (p presenter) auditCard(l, server string, findings []guardian.Finding, webhooksChecked bool) discord.ContainerComponent {
	lines := []string{"### " + strings.TrimSpace(p.emojis.Get(EmojiShield)+" "+p.t(l, i18n.GuardianAuditTitle, map[string]any{"Server": escapeMarkdown(server)}))}
	if len(findings) == 0 {
		lines = append(lines, p.emojis.Get(EmojiOK)+" "+p.t(l, i18n.GuardianAuditClean, nil))
	}
	const maxShown = 20
	for i, f := range findings {
		if i == maxShown {
			lines = append(lines, fmt.Sprintf("… +%d", len(findings)-maxShown))
			break
		}
		lines = append(lines, fmt.Sprintf("%s **%s** · %s", severityEmoji(f.Severity), p.t(l, i18n.GuardianSeverity(string(f.Severity)), nil), p.auditText(l, f)))
	}
	if !webhooksChecked {
		lines = append(lines, "🔵 "+p.t(l, i18n.GuardianAuditWebhooksSkip, nil))
	}
	lines = append(lines, "-# "+p.t(l, i18n.GuardianAuditFooter, nil))
	color := colorSuccess
	if len(findings) > 0 {
		color = colorWarning
		if findings[0].Severity == guardian.SeverityCritical || findings[0].Severity == guardian.SeverityHigh {
			color = colorDanger
		}
	}
	text := strings.Join(lines, "\n")
	if len(text) > 3900 { // Components V2 text limit is 4000
		text = text[:3900] + "…"
	}
	return discord.NewContainer(discord.NewTextDisplay(text)).WithAccentColor(color)
}
