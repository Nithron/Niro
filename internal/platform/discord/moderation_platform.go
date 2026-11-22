package discord

import (
	"context"
	"errors"
	"time"

	"github.com/disgoorg/disgo/bot"
	"github.com/disgoorg/disgo/discord"
	"github.com/disgoorg/disgo/rest"
	"github.com/disgoorg/omit"
	"github.com/disgoorg/snowflake/v2"

	"github.com/nithron/nithro/internal/core"
	"github.com/nithron/nithro/internal/modules/moderation"
)

// modPlatform implements moderation.Platform on Discord.
type modPlatform struct{ client *bot.Client }

var _ moderation.Platform = modPlatform{}

func (p modPlatform) Member(ctx context.Context, guildID, userID snowflake.ID) (moderation.Member, error) {
	if m, ok := p.client.Caches.Member(guildID, userID); ok {
		return p.toMember(guildID, m), nil
	}
	m, err := p.client.Rest.GetMember(guildID, userID, rest.WithCtx(ctx))
	if rest.IsJSONErrorCode(err, rest.JSONErrorCodeUnknownMember, rest.JSONErrorCodeUnknownUser) {
		return moderation.Member{}, moderation.ErrNotMember
	}
	if err != nil {
		return moderation.Member{}, err
	}
	return p.toMember(guildID, *m), nil
}

func (p modPlatform) BotMember(ctx context.Context, guildID snowflake.ID) (moderation.Member, error) {
	self, ok := p.client.Caches.SelfUser()
	if !ok {
		return moderation.Member{}, errors.New("bot user not known yet")
	}
	return p.Member(ctx, guildID, self.ID)
}

func (p modPlatform) toMember(guildID snowflake.ID, m discord.Member) moderation.Member {
	return moderation.Member{
		Rank:          rankOf(p.client, guildID, m.User.ID, m.RoleIDs),
		Name:          displayName(m.User, m.Nick),
		IsBot:         m.User.Bot,
		TimedOutUntil: m.CommunicationDisabledUntil,
	}
}

// rankOf computes a member's hierarchy rank from the cached guild and roles.
func rankOf(client *bot.Client, guildID, userID snowflake.ID, roleIDs []snowflake.ID) core.Rank {
	r := core.Rank{ID: userID}
	if g, ok := client.Caches.Guild(guildID); ok && g.OwnerID == userID {
		r.IsOwner = true
	}
	for _, id := range roleIDs {
		if role, ok := client.Caches.Role(guildID, id); ok && role.Position > r.TopRolePosition {
			r.TopRolePosition = role.Position
		}
	}
	return r
}

func displayName(u discord.User, nick *string) string {
	switch {
	case nick != nil && *nick != "":
		return *nick
	case u.GlobalName != nil && *u.GlobalName != "":
		return *u.GlobalName
	}
	return u.Username
}

func (p modPlatform) Timeout(ctx context.Context, guildID, userID snowflake.ID, until time.Time, reason string) error {
	_, err := p.client.Rest.UpdateMember(guildID, userID, discord.MemberUpdate{
		CommunicationDisabledUntil: omit.New(&until),
	}, rest.WithCtx(ctx), rest.WithReason(reason))
	return err
}

func (p modPlatform) RemoveTimeout(ctx context.Context, guildID, userID snowflake.ID, reason string) error {
	_, err := p.client.Rest.UpdateMember(guildID, userID, discord.MemberUpdate{
		CommunicationDisabledUntil: omit.New[*time.Time](nil),
	}, rest.WithCtx(ctx), rest.WithReason(reason))
	return err
}

func (p modPlatform) Kick(ctx context.Context, guildID, userID snowflake.ID, reason string) error {
	return p.client.Rest.RemoveMember(guildID, userID, rest.WithCtx(ctx), rest.WithReason(reason))
}

func (p modPlatform) Ban(ctx context.Context, guildID, userID snowflake.ID, deleteMessages time.Duration, reason string) error {
	return p.client.Rest.AddBan(guildID, userID, deleteMessages, rest.WithCtx(ctx), rest.WithReason(reason))
}

func (p modPlatform) Unban(ctx context.Context, guildID, userID snowflake.ID, reason string) error {
	err := p.client.Rest.DeleteBan(guildID, userID, rest.WithCtx(ctx), rest.WithReason(reason))
	if rest.IsJSONErrorCode(err, rest.JSONErrorCodeUnknownBan) {
		return moderation.ErrNotBanned
	}
	return err
}

func (p modPlatform) RecentMessages(ctx context.Context, channelID, before snowflake.ID, limit int) ([]moderation.Message, error) {
	msgs, err := p.client.Rest.GetMessages(channelID, 0, before, 0, limit, rest.WithCtx(ctx))
	if err != nil {
		return nil, err
	}
	out := make([]moderation.Message, len(msgs))
	for i, m := range msgs {
		out[i] = moderation.Message{
			ID: m.ID, AuthorID: m.Author.ID, AuthorBot: m.Author.Bot, Content: m.Content,
			HasEmbeds: len(m.Embeds) > 0, Pinned: m.Pinned, CreatedAt: m.CreatedAt,
		}
	}
	return out, nil
}

func (p modPlatform) DeleteMessages(ctx context.Context, channelID snowflake.ID, ids []snowflake.ID, reason string) error {
	if len(ids) == 1 {
		return p.client.Rest.DeleteMessage(channelID, ids[0], rest.WithCtx(ctx), rest.WithReason(reason))
	}
	for start := 0; start < len(ids); start += 100 {
		end := min(start+100, len(ids))
		chunk := ids[start:end]
		var err error
		if len(chunk) == 1 {
			err = p.client.Rest.DeleteMessage(channelID, chunk[0], rest.WithCtx(ctx), rest.WithReason(reason))
		} else {
			err = p.client.Rest.BulkDeleteMessages(channelID, chunk, rest.WithCtx(ctx), rest.WithReason(reason))
		}
		if err != nil {
			return err
		}
	}
	return nil
}
