package discord

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/disgoorg/disgo/bot"
	"github.com/disgoorg/disgo/discord"
	"github.com/disgoorg/disgo/rest"
	"github.com/disgoorg/snowflake/v2"

	"github.com/nithron/niro/internal/i18n"
	"github.com/nithron/niro/internal/modules/moderation"
)

const (
	colorWarning = 0xF5B83D
	colorOrange  = 0xFF8A4C
)

// actionColor is the accent colour of a case card.
func actionColor(a moderation.Action) int {
	switch a {
	case moderation.ActionWarn, moderation.ActionTimeout:
		return colorWarning
	case moderation.ActionKick:
		return colorOrange
	case moderation.ActionBan:
		return colorDanger
	case moderation.ActionUntimeout, moderation.ActionUnban:
		return colorSuccess
	}
	return colorBrand
}

func actionEmoji(e *Emojis, a moderation.Action) string {
	switch a {
	case moderation.ActionWarn, moderation.ActionTimeout:
		return e.Get(EmojiWarn)
	case moderation.ActionKick, moderation.ActionBan:
		return e.Get(EmojiShield)
	case moderation.ActionUntimeout, moderation.ActionUnban:
		return e.Get(EmojiOK)
	}
	return e.Get(EmojiInfo)
}

// presenter renders moderation objects as Discord messages.
type presenter struct {
	cat    *i18n.Catalog
	emojis *Emojis
}

func (p presenter) t(locale string, k i18n.Key, data map[string]any) string {
	return p.cat.T(locale, k, data)
}

func userRef(id snowflake.ID, name string) string {
	if id == 0 {
		return "—"
	}
	if name == "" {
		return fmt.Sprintf("<@%s> (`%s`)", id, id)
	}
	return fmt.Sprintf("<@%s> (%s · `%s`)", id, escapeMarkdown(name), id)
}

func ts(t time.Time, style string) string { return fmt.Sprintf("<t:%d:%s>", t.Unix(), style) }

// caseCard renders a case for the moderation log and for /case view.
func (p presenter) caseCard(locale string, c moderation.Case) discord.ContainerComponent {
	title := p.t(locale, i18n.ModLogTitle, map[string]any{"Number": c.Number, "Action": p.t(locale, i18n.ModAction(string(c.Action)), nil)})
	head := "### " + strings.TrimSpace(actionEmoji(p.emojis, c.Action)+" "+title)
	if c.Deleted {
		head = "### ~~" + title + "~~ (" + p.t(locale, i18n.ModLogDeleted, nil) + ")"
	}

	var lines []string
	if c.Action == moderation.ActionPurge {
		lines = append(lines, p.t(locale, i18n.ModLogPurge, map[string]any{
			"Channel": fmt.Sprintf("<#%s>", c.Details.ChannelID), "Count": c.Details.Deleted,
		}))
		if c.TargetID != 0 {
			lines = append(lines, p.t(locale, i18n.ModLogUser, map[string]any{"User": userRef(c.TargetID, c.TargetName)}))
		}
	} else {
		lines = append(lines, p.t(locale, i18n.ModLogUser, map[string]any{"User": userRef(c.TargetID, c.TargetName)}))
	}
	lines = append(lines, p.t(locale, i18n.ModLogModerator, map[string]any{"Moderator": userRef(c.ModeratorID, c.ModeratorName)}))
	reason := p.t(locale, i18n.ModLogNoReason, nil)
	if c.Reason != "" {
		reason = escapeMarkdown(c.Reason)
	}
	lines = append(lines, p.t(locale, i18n.ModLogReason, map[string]any{"Reason": reason}))
	if c.Duration > 0 && c.ExpiresAt != nil {
		lines = append(lines, p.t(locale, i18n.ModLogDuration, map[string]any{
			"Duration": moderation.FormatDuration(c.Duration), "Ends": ts(*c.ExpiresAt, "R"),
		}))
	}
	if c.Details.DeleteMessages > 0 {
		lines = append(lines, p.t(locale, i18n.ModLogBanDeleted, map[string]any{
			"Duration": moderation.FormatDuration(time.Duration(c.Details.DeleteMessages) * time.Second),
		}))
	}

	footer := []string{ts(c.CreatedAt, "f")}
	switch c.Source {
	case moderation.SourceAutoMod:
		footer = append(footer, p.t(locale, i18n.ModLogSourceAutomod, nil))
	case moderation.SourceGuardian:
		footer = append(footer, p.t(locale, i18n.ModLogSourceGuardian, nil))
	case moderation.SourceDashboard:
		footer = append(footer, p.t(locale, i18n.ModLogSourceDashboard, nil))
	case moderation.SourceSystem:
		footer = append(footer, p.t(locale, i18n.ModLogSourceSystem, nil))
	}
	if notifiable(c.Action) {
		if c.DMSent {
			footer = append(footer, p.t(locale, i18n.ModLogDmSent, nil))
		} else {
			footer = append(footer, p.t(locale, i18n.ModLogDmNotSent, nil))
		}
	}

	color := actionColor(c.Action)
	if c.Deleted {
		color = 0x5B6576
	}
	return discord.NewContainer(
		discord.NewTextDisplay(head+"\n"+strings.Join(lines, "\n")),
		discord.NewSmallSeparator(),
		discord.NewTextDisplay("-# "+strings.Join(footer, " · ")),
	).WithAccentColor(color)
}

func notifiable(a moderation.Action) bool {
	return a == moderation.ActionWarn || a == moderation.ActionTimeout || a == moderation.ActionKick || a == moderation.ActionBan
}

// dmCard renders the private message sent to the affected user.
func (p presenter) dmCard(locale string, c moderation.Case, guildName string) discord.ContainerComponent {
	var key i18n.Key
	switch c.Action {
	case moderation.ActionWarn:
		key = i18n.ModDmWarn
	case moderation.ActionTimeout:
		key = i18n.ModDmTimeout
	case moderation.ActionKick:
		key = i18n.ModDmKick
	default:
		key = i18n.ModDmBan
	}
	lines := []string{"### " + p.t(locale, key, map[string]any{"Server": escapeMarkdown(guildName)})}
	if c.Reason != "" {
		lines = append(lines, p.t(locale, i18n.ModDmReason, map[string]any{"Reason": escapeMarkdown(c.Reason)}))
	} else {
		lines = append(lines, p.t(locale, i18n.ModDmNoReason, nil))
	}
	if c.ExpiresAt != nil {
		lines = append(lines, p.t(locale, i18n.ModDmUntil, map[string]any{"Until": ts(*c.ExpiresAt, "f") + " (" + ts(*c.ExpiresAt, "R") + ")"}))
	}
	return discord.NewContainer(
		discord.NewTextDisplay(strings.Join(lines, "\n")),
		discord.NewSmallSeparator(),
		discord.NewTextDisplay("-# "+p.t(locale, i18n.ModDmFooter, nil)),
	).WithAccentColor(actionColor(c.Action))
}

var mdEscaper = strings.NewReplacer(`\`, `\\`, `*`, `\*`, `_`, `\_`, "`", "\\`", `~`, `\~`, `|`, `\|`, `>`, `\>`, `#`, `\#`, `[`, `\[`, `]`, `\]`)

// escapeMarkdown prevents user-supplied text (names, reasons) from breaking
// or spoofing message formatting.
func escapeMarkdown(s string) string { return mdEscaper.Replace(s) }

// noPings lets mentions render as names without notifying anyone.
var noPings = &discord.AllowedMentions{}

// modNotifier implements moderation.Notifier on Discord.
type modNotifier struct {
	client *bot.Client
	p      presenter
	locale func(guildID snowflake.ID) string
}

var _ moderation.Notifier = modNotifier{}

func (n modNotifier) NotifyUser(ctx context.Context, c moderation.Case, guildName string) error {
	ch, err := n.client.Rest.CreateDMChannel(c.TargetID, rest.WithCtx(ctx))
	if err != nil {
		return err
	}
	_, err = n.client.Rest.CreateMessage(ch.ID(), discord.MessageCreate{
		Flags:           discord.MessageFlagIsComponentsV2,
		Components:      []discord.LayoutComponent{n.p.dmCard(n.locale(c.GuildID), c, guildName)},
		AllowedMentions: noPings,
	}, rest.WithCtx(ctx))
	return err
}

func (n modNotifier) PostLog(ctx context.Context, channelID snowflake.ID, c moderation.Case) (snowflake.ID, error) {
	msg, err := n.client.Rest.CreateMessage(channelID, discord.MessageCreate{
		Flags:           discord.MessageFlagIsComponentsV2,
		Components:      []discord.LayoutComponent{n.p.caseCard(n.locale(c.GuildID), c)},
		AllowedMentions: noPings,
	}, rest.WithCtx(ctx))
	if err != nil {
		return 0, err
	}
	return msg.ID, nil
}

func (n modNotifier) EditLog(ctx context.Context, channelID, messageID snowflake.ID, c moderation.Case) error {
	components := []discord.LayoutComponent{n.p.caseCard(n.locale(c.GuildID), c)}
	_, err := n.client.Rest.UpdateMessage(channelID, messageID, discord.MessageUpdate{
		Components:      &components,
		AllowedMentions: noPings,
	}, rest.WithCtx(ctx))
	return err
}
