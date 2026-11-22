package discord

import (
	"errors"
	"fmt"
	"net/url"
	"strings"
	"time"

	"github.com/disgoorg/disgo/discord"
	"github.com/disgoorg/disgo/handler"

	"github.com/nithron/nithro/internal/i18n"
)

// Brand colours (assets/brand/palette.json).
const (
	colorBrand   = 0x4CB8FF
	colorSuccess = 0x3DDBA6
	colorDanger  = 0xFF6B6B
)

var errNotInGuild = errors.New("interaction outside a guild")

func (b *Bot) t(locale discord.Locale, k i18n.Key, data map[string]any) string {
	return b.deps.Catalog.T(locale.Code(), k, data)
}

func ephemeralV2() discord.MessageFlags {
	return discord.MessageFlagIsComponentsV2 | discord.MessageFlagEphemeral
}

func (b *Bot) handlePing(_ discord.SlashCommandInteractionData, e *handler.CommandEvent) error {
	latency := b.client.ShardManager.Shard(e.ShardID()).Latency()
	text := b.t(e.Locale(), i18n.CmdPingReply, map[string]any{"Latency": formatLatency(latency)})
	return e.CreateMessage(discord.MessageCreate{
		Flags: ephemeralV2(),
		Components: []discord.LayoutComponent{
			discord.NewContainer(discord.NewTextDisplay(b.emojis.Get(EmojiOK) + " " + text)).WithAccentColor(colorSuccess),
		},
	})
}

func (b *Bot) handleAbout(_ discord.SlashCommandInteractionData, e *handler.CommandEvent) error {
	loc := e.Locale()
	title := strings.TrimSpace(b.emojis.Get(EmojiNithro) + " " + b.t(loc, i18n.CmdAboutTitle, nil))
	body := b.t(loc, i18n.CmdAboutBody, map[string]any{"Version": b.deps.Version})

	container := discord.NewContainer(
		discord.NewTextDisplay("## " + title + "\n" + body),
	).WithAccentColor(colorBrand)
	container = withLinks(container,
		link{b.t(loc, i18n.CmdAboutDashboard, nil), b.deps.DashboardURL.String()},
		link{b.t(loc, i18n.CmdAboutWebsite, nil), "https://nithron.com"},
	)
	return e.CreateMessage(discord.MessageCreate{Flags: ephemeralV2(), Components: []discord.LayoutComponent{container}})
}

func (b *Bot) handleModules(_ discord.SlashCommandInteractionData, e *handler.CommandEvent) error {
	guildID := e.GuildID()
	if guildID == nil {
		return errNotInGuild
	}
	loc := e.Locale()
	states, err := b.deps.Config.ModuleStates(e.Ctx, *guildID)
	if err != nil {
		return fmt.Errorf("loading module states: %w", err)
	}

	serverName := guildID.String()
	if g, ok := e.Guild(); ok {
		serverName = g.Name
	}
	var lines []string
	for _, st := range states {
		id := string(st.Module.ID)
		emoji, status := b.emojis.Get(EmojiOK), b.t(loc, i18n.CmdModulesEnabled, nil)
		switch {
		case st.Module.AlwaysOn:
			status = b.t(loc, i18n.CmdModulesAlwaysOn, nil)
		case !st.Enabled:
			emoji, status = b.emojis.Get(EmojiError), b.t(loc, i18n.CmdModulesDisabled, nil)
		}
		lines = append(lines, fmt.Sprintf("%s **%s** · %s\n-# %s",
			emoji, b.t(loc, i18n.ModuleName(id), nil), status, b.t(loc, i18n.ModuleDescription(id), nil)))
	}

	container := discord.NewContainer(
		discord.NewTextDisplay("## "+b.t(loc, i18n.CmdModulesTitle, map[string]any{"Server": serverName})),
		discord.NewTextDisplay(strings.Join(lines, "\n")),
		discord.NewSmallSeparator(),
		discord.NewTextDisplay("-# "+b.t(loc, i18n.CmdModulesFooter, nil)),
	).WithAccentColor(colorBrand)
	dashboard := b.deps.DashboardURL.JoinPath("guilds", guildID.String()).String()
	container = withLinks(container, link{b.t(loc, i18n.CmdModulesManage, nil), dashboard})

	return e.CreateMessage(discord.MessageCreate{Flags: ephemeralV2(), Components: []discord.LayoutComponent{container}})
}

func (b *Bot) handleError(e *handler.InteractionEvent, err error) {
	key := i18n.ErrorGeneric
	if errors.Is(err, errNotInGuild) {
		key = i18n.ErrorGuildOnly
	} else {
		b.log.Error("interaction failed", "name", interactionName(e.Interaction), "guild_id", e.GuildID(), "err", err)
	}
	msg := discord.MessageCreate{
		Flags: ephemeralV2(),
		Components: []discord.LayoutComponent{
			discord.NewContainer(discord.NewTextDisplay(b.emojis.Get(EmojiError) + " " + b.t(e.Locale(), key, nil))).WithAccentColor(colorDanger),
		},
	}
	if rerr := e.CreateMessage(msg); rerr != nil {
		b.log.Warn("sending error reply failed", "err", rerr)
	}
}

type link struct{ label, url string }

// withLinks appends link buttons to a container. Discord only accepts public
// https URLs in buttons, so local development URLs are shown as text links.
func withLinks(c discord.ContainerComponent, links ...link) discord.ContainerComponent {
	var buttons []discord.InteractiveComponent
	var text []string
	for _, l := range links {
		if publicHTTPS(l.url) {
			buttons = append(buttons, discord.NewLinkButton(l.label, l.url))
		} else {
			text = append(text, fmt.Sprintf("[%s](%s)", l.label, l.url))
		}
	}
	if len(text) > 0 {
		c = c.AddComponents(discord.NewTextDisplay(strings.Join(text, " · ")))
	}
	if len(buttons) > 0 {
		c = c.AddComponents(discord.NewActionRow(buttons...))
	}
	return c
}

func publicHTTPS(raw string) bool {
	u, err := url.Parse(raw)
	if err != nil || u.Scheme != "https" {
		return false
	}
	h := u.Hostname()
	return h != "localhost" && h != "127.0.0.1" && h != "::1"
}

func formatLatency(d time.Duration) string {
	if d <= 0 {
		return "—"
	}
	return fmt.Sprintf("%d ms", d.Milliseconds())
}
