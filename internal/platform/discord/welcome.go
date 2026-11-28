package discord

import (
	"context"
	"time"

	"github.com/disgoorg/disgo/bot"
	"github.com/disgoorg/disgo/discord"
	"github.com/disgoorg/disgo/events"
	"github.com/disgoorg/disgo/rest"
	"github.com/disgoorg/snowflake/v2"

	"github.com/nithron/niro/internal/core/message"
	"github.com/nithron/niro/internal/i18n"
	"github.com/nithron/niro/internal/modules/welcome"
)

// welcomePlatform implements welcome.Platform on Discord.
type welcomePlatform struct {
	rolesPlatform
}

var _ welcome.Platform = welcomePlatform{}

func (w welcomePlatform) Server(_ context.Context, guildID snowflake.ID) (string, int) {
	if g, ok := w.client.Caches.Guild(guildID); ok {
		return g.Name, g.MemberCount
	}
	return "", 0
}

func (w welcomePlatform) Post(ctx context.Context, channelID snowflake.ID, d message.Document, ping snowflake.ID) error {
	mentions := noPings
	if ping != 0 {
		// Only the new member may be notified, never @everyone or roles.
		mentions = &discord.AllowedMentions{Users: []snowflake.ID{ping}}
	}
	_, err := w.client.Rest.CreateMessage(channelID, documentMessage(renderDocument(d), mentions), rest.WithCtx(ctx))
	return err
}

func (w welcomePlatform) DM(ctx context.Context, userID snowflake.ID, d message.Document) error {
	ch, err := w.client.Rest.CreateDMChannel(userID, rest.WithCtx(ctx))
	if err != nil {
		return err
	}
	_, err = w.client.Rest.CreateMessage(ch.ID(), documentMessage(renderDocument(d), nil), rest.WithCtx(ctx))
	return err
}

func (welcomePlatform) UserMention(id snowflake.ID) string    { return "<@" + id.String() + ">" }
func (welcomePlatform) ChannelMention(id snowflake.ID) string { return "<#" + id.String() + ">" }
func (welcomePlatform) Escape(s string) string                { return escapeMarkdown(s) }

func newWelcomePlatform(client *bot.Client, p presenter, locale func(snowflake.ID) string) welcomePlatform {
	return welcomePlatform{rolesPlatform{client: client, p: p, locale: locale}}
}

// raidJoinSummary posts one line for the joins Niro didn't greet during a raid.
func (b *Bot) raidJoinSummary(ctx context.Context, channelID, guildID snowflake.ID, joins int) error {
	text := b.deps.Catalog.T(b.guildLocale(guildID), i18n.WelcomeRaidSummary, map[string]any{"Count": joins})
	_, err := b.client.Rest.CreateMessage(channelID, documentMessage(b.infoCard("", EmojiShield, text, barEarlier), nil), rest.WithCtx(ctx))
	return err
}

func (b *Bot) welcomeGate(ctx context.Context, guildID, userID snowflake.ID, at time.Time) welcome.Gate {
	g := b.guard.JoinGate(ctx, guildID, userID, at)
	return welcome.Gate{Blocked: g.Blocked, Raid: g.Raid, VerifyRoleID: g.VerifyRoleID, Unknown: g.Unknown}
}

// welcomeMember describes a join for the welcome module (run after Guardian,
// see onMemberJoin).
func welcomeMember(e *events.GuildMemberJoin) welcome.Member {
	u := e.Member.User
	m := welcome.Member{GuildID: e.GuildID, UserID: u.ID, Name: displayName(u, e.Member.Nick), Bot: u.Bot, At: time.Now()}
	if e.Member.JoinedAt != nil {
		m.At = *e.Member.JoinedAt
	}
	return m
}

func (b *Bot) onWelcomeLeave(e *events.GuildMemberLeave) {
	m := welcome.Member{GuildID: e.GuildID, UserID: e.User.ID, Name: e.User.EffectiveName(), Bot: e.User.Bot, At: time.Now()}
	if e.Member.User.ID != 0 {
		m.Name = displayName(e.User, e.Member.Nick)
	}
	b.logRun("goodbye", func(ctx context.Context) error { return b.welcome.HandleLeave(ctx, m) })
}
