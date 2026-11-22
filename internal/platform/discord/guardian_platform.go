package discord

import (
	"context"
	"errors"
	"slices"
	"time"

	"github.com/disgoorg/disgo/bot"
	"github.com/disgoorg/disgo/discord"
	"github.com/disgoorg/disgo/rest"
	"github.com/disgoorg/omit"
	"github.com/disgoorg/snowflake/v2"

	"github.com/nithron/niro/internal/modules/guardian"
	"github.com/nithron/niro/internal/modules/moderation"
)

// guardianPlatform implements guardian.Platform on Discord. Member actions
// shared with moderation (timeouts, kicks, deletes) reuse modPlatform.
type guardianPlatform struct {
	modPlatform
}

var _ guardian.Platform = guardianPlatform{}

func newGuardianPlatform(client *bot.Client) guardianPlatform {
	return guardianPlatform{modPlatform{client: client}}
}

func (p guardianPlatform) GuildName(guildID snowflake.ID) string {
	if g, ok := p.client.Caches.Guild(guildID); ok {
		return g.Name
	}
	return ""
}

func (p guardianPlatform) Member(ctx context.Context, guildID, userID snowflake.ID) (guardian.Member, error) {
	m, ok := p.client.Caches.Member(guildID, userID)
	if !ok {
		rm, err := p.client.Rest.GetMember(guildID, userID, rest.WithCtx(ctx))
		if rest.IsJSONErrorCode(err, rest.JSONErrorCodeUnknownMember, rest.JSONErrorCodeUnknownUser) {
			return guardian.Member{}, moderation.ErrNotMember
		}
		if err != nil {
			return guardian.Member{}, err
		}
		m = *rm
	}
	return guardian.Member{
		Rank:          rankOf(p.client, guildID, m.User.ID, m.RoleIDs),
		Name:          displayName(m.User, m.Nick),
		IsBot:         m.User.Bot,
		RoleIDs:       slices.Clone(m.RoleIDs),
		TimedOutUntil: m.CommunicationDisabledUntil,
	}, nil
}

func (p guardianPlatform) BotMember(ctx context.Context, guildID snowflake.ID) (guardian.Member, error) {
	self, ok := p.client.Caches.SelfUser()
	if !ok {
		return guardian.Member{}, errors.New("bot user not known yet")
	}
	m, err := p.Member(ctx, guildID, self.ID)
	if err == nil && m.Name == "" {
		m.Name = self.Username
	}
	return m, err
}

// SetRoles replaces a member's roles. Roles Discord manages (bot roles, the
// booster role) cannot be removed, so they are always kept; roles that no
// longer exist are dropped so a restore never fails on them.
func (p guardianPlatform) SetRoles(ctx context.Context, guildID, userID snowflake.ID, roleIDs []snowflake.ID, reason string) error {
	current, err := p.Member(ctx, guildID, userID)
	if err != nil {
		return err
	}
	var want []snowflake.ID
	for _, id := range current.RoleIDs {
		if r, ok := p.client.Caches.Role(guildID, id); ok && r.Managed {
			want = append(want, id)
		}
	}
	for _, id := range roleIDs {
		if _, ok := p.client.Caches.Role(guildID, id); ok && !slices.Contains(want, id) {
			want = append(want, id)
		}
	}
	if want == nil {
		want = []snowflake.ID{}
	}
	_, err = p.client.Rest.UpdateMember(guildID, userID, discord.MemberUpdate{Roles: &want}, rest.WithCtx(ctx), rest.WithReason(reason))
	return err
}

func (p guardianPlatform) PauseInvites(ctx context.Context, guildID snowflake.ID, until time.Time) error {
	var t *time.Time
	if !until.IsZero() {
		t = &until
	}
	_, err := p.client.Rest.UpdateGuildIncidentActions(guildID, discord.GuildIncidentActionsUpdate{
		InvitesDisabledUntil: omit.New(t),
	}, rest.WithCtx(ctx))
	return err
}

func (p guardianPlatform) VerificationLevel(ctx context.Context, guildID snowflake.ID) (int, error) {
	if g, ok := p.client.Caches.Guild(guildID); ok {
		return int(g.VerificationLevel), nil
	}
	g, err := p.client.Rest.GetGuild(guildID, false, rest.WithCtx(ctx))
	if err != nil {
		return 0, err
	}
	return int(g.VerificationLevel), nil
}

func (p guardianPlatform) SetVerificationLevel(ctx context.Context, guildID snowflake.ID, level int, reason string) error {
	lvl := discord.VerificationLevel(level)
	_, err := p.client.Rest.UpdateGuild(guildID, discord.GuildUpdate{VerificationLevel: omit.New(&lvl)}, rest.WithCtx(ctx), rest.WithReason(reason))
	return err
}

// TextChannels lists text and announcement channels from the cache.
func (p guardianPlatform) TextChannels(_ context.Context, guildID snowflake.ID) ([]guardian.Channel, error) {
	var out []guardian.Channel
	for ch := range p.client.Caches.ChannelsForGuild(guildID) {
		switch c := ch.(type) {
		case discord.GuildTextChannel:
			out = append(out, guardian.Channel{ID: c.ID(), Slowmode: c.RateLimitPerUser()})
		}
	}
	slices.SortFunc(out, func(a, b guardian.Channel) int { return int(a.ID.Time().Sub(b.ID.Time())) })
	return out, nil
}

func (p guardianPlatform) SetSlowmode(ctx context.Context, channelID snowflake.ID, seconds int, reason string) error {
	_, err := p.client.Rest.UpdateChannel(channelID, discord.GuildTextChannelUpdate{RateLimitPerUser: &seconds}, rest.WithCtx(ctx), rest.WithReason(reason))
	return err
}

// DeleteMessages deletes flagged copies. Messages already gone (deleted by
// the author, AutoMod or a parallel detection) count as done.
func (p guardianPlatform) DeleteMessages(ctx context.Context, channelID snowflake.ID, ids []snowflake.ID, reason string) error {
	err := p.modPlatform.DeleteMessages(ctx, channelID, ids, reason)
	if err == nil || rest.IsJSONErrorCode(err, rest.JSONErrorCodeUnknownMessage) && len(ids) == 1 {
		return nil
	}
	if len(ids) == 1 {
		return err
	}
	// A bulk delete fails as a whole; retry one by one so the rest still go.
	var firstErr error
	for _, id := range ids {
		e := p.client.Rest.DeleteMessage(channelID, id, rest.WithCtx(ctx), rest.WithReason(reason))
		if e != nil && !rest.IsJSONErrorCode(e, rest.JSONErrorCodeUnknownMessage) && firstErr == nil {
			firstErr = e
		}
	}
	return firstErr
}
