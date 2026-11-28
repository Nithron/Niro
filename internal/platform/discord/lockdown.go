package discord

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/disgoorg/disgo/discord"
	"github.com/disgoorg/disgo/handler"
	"github.com/disgoorg/disgo/rest"
	"github.com/disgoorg/snowflake/v2"

	"github.com/nithron/niro/internal/i18n"
	"github.com/nithron/niro/internal/modules/guardian"
)

// lockBits are denied to @everyone during a lockdown.
const lockBits = discord.PermissionSendMessages | discord.PermissionSendMessagesInThreads |
	discord.PermissionCreatePublicThreads | discord.PermissionCreatePrivateThreads | discord.PermissionAddReactions

// maxLockChannels caps a server-wide lockdown.
const maxLockChannels = 200

// lockable returns a text or announcement channel's @everyone overwrite.
func (b *Bot) lockable(guildID, channelID snowflake.ID) (discord.RolePermissionOverwrite, bool, bool) {
	ch, ok := b.client.Caches.Channel(channelID)
	if !ok {
		return discord.RolePermissionOverwrite{}, false, false
	}
	var ows discord.PermissionOverwrites
	switch c := ch.(type) {
	case discord.GuildTextChannel:
		ows = c.PermissionOverwrites()
	case discord.GuildNewsChannel:
		ows = c.PermissionOverwrites()
	default:
		return discord.RolePermissionOverwrite{}, false, false
	}
	ow, has := ows.Role(guildID)
	return ow, has, true
}

// lockChannel denies sending to @everyone and saves the previous overwrite.
// It returns false if the channel already denies sending.
func (b *Bot) lockChannel(ctx context.Context, guildID, channelID, by snowflake.ID, reason string) (bool, error) {
	ow, has, ok := b.lockable(guildID, channelID)
	if !ok {
		return false, errBadChannel
	}
	if ow.Deny.Has(discord.PermissionSendMessages) {
		return false, nil
	}
	if err := b.guardianStore().SaveLockdowns(ctx, []guardian.LockState{{
		GuildID: guildID, ChannelID: channelID, HadOverwrite: has, Allow: int64(ow.Allow), Deny: int64(ow.Deny),
		By: by, CreatedAt: time.Now(),
	}}); err != nil {
		return false, err
	}
	allow, deny := ow.Allow.Remove(lockBits), ow.Deny.Add(lockBits)
	return true, b.client.Rest.UpdatePermissionOverwrite(channelID, guildID,
		discord.RolePermissionOverwriteUpdate{Allow: &allow, Deny: &deny}, rest.WithCtx(ctx), rest.WithReason(reason))
}

// unlockChannel restores the saved overwrite exactly.
func (b *Bot) unlockChannel(ctx context.Context, st guardian.LockState, reason string) error {
	var err error
	if st.HadOverwrite {
		allow, deny := discord.Permissions(st.Allow), discord.Permissions(st.Deny)
		err = b.client.Rest.UpdatePermissionOverwrite(st.ChannelID, st.GuildID,
			discord.RolePermissionOverwriteUpdate{Allow: &allow, Deny: &deny}, rest.WithCtx(ctx), rest.WithReason(reason))
	} else {
		err = b.client.Rest.DeletePermissionOverwrite(st.ChannelID, st.GuildID, rest.WithCtx(ctx), rest.WithReason(reason))
		if rest.IsJSONErrorCode(err, rest.JSONErrorCodeUnknownPermissionOverwrite) {
			err = nil
		}
	}
	if rest.IsJSONErrorCode(err, rest.JSONErrorCodeUnknownChannel) {
		err = nil // channel deleted meanwhile
	}
	if err != nil {
		return err
	}
	return b.guardianStore().DeleteLockdowns(ctx, st.GuildID, []snowflake.ID{st.ChannelID})
}

var errBadChannel = fmt.Errorf("not a text channel")

func auditText(name, reason string) string {
	r := name + ": lockdown"
	if strings.TrimSpace(reason) != "" {
		r += " (" + strings.TrimSpace(reason) + ")"
	}
	return truncateRunes(r, 512)
}

func truncateRunes(s string, n int) string {
	r := []rune(s)
	if len(r) <= n {
		return s
	}
	return string(r[:n])
}

// postNotice posts a short notice and returns its ID (0 if it failed).
func (b *Bot) postNotice(channelID snowflake.ID, text string) snowflake.ID {
	m, err := b.client.Rest.CreateMessage(channelID, discord.MessageCreate{
		Flags:           discord.MessageFlagIsComponentsV2,
		Components:      []discord.LayoutComponent{discord.NewContainer(discord.NewTextDisplay(text)).WithAccentColor(barEarlier)},
		AllowedMentions: noPings,
	})
	if err != nil {
		return 0
	}
	return m.ID
}

func (b *Bot) handleLockdownChannel(data discord.SlashCommandInteractionData, e *handler.CommandEvent) error {
	mc, ok := b.modCtx(e)
	if !ok {
		return errNotInGuild
	}
	if on, err := b.guardianOn(e, mc); !on || err != nil {
		return err
	}
	l := discord.Locale(mc.locale)
	chID := e.Channel().ID()
	if c, ok := data.OptChannel("channel"); ok {
		chID = c.ID
	}
	reason, _ := data.OptString("reason")
	mention := "<#" + chID.String() + ">"

	if _, _, ok := b.lockable(mc.guildID, chID); !ok {
		return b.replyNow(e, b.infoCard(mc.locale, EmojiError, b.t(l, i18n.LockdownBadChannel, nil), barFailed))
	}
	if ow, _, _ := b.lockable(mc.guildID, chID); ow.Deny.Has(discord.PermissionSendMessages) {
		return b.replyNow(e, b.infoCard(mc.locale, EmojiInfo, b.t(l, i18n.LockdownAlready, map[string]any{"Channel": mention}), barNone))
	}
	if err := e.DeferCreateMessage(true); err != nil {
		return err
	}
	notice := b.postNotice(chID, b.lockNotice(mc.guildID, reason))
	if _, err := b.lockChannel(e.Ctx, mc.guildID, chID, mc.actor.ID, auditText(mc.actor.Name, reason)); err != nil {
		b.log.Warn("lockdown failed", "channel_id", chID, "err", err)
		if notice != 0 {
			_ = b.client.Rest.DeleteMessage(chID, notice)
		}
		return replyDeferred(e.UpdateInteractionResponse, b.infoCard(mc.locale, EmojiError, b.t(l, i18n.LockdownFailed, map[string]any{"Count": 1}), barFailed))
	}
	return replyDeferred(e.UpdateInteractionResponse, b.infoCard(mc.locale, EmojiOK, b.t(l, i18n.LockdownDoneChannel, map[string]any{"Channel": mention}), barPass))
}

func (b *Bot) lockNotice(guildID snowflake.ID, reason string) string {
	gl := b.guildLocale(guildID)
	text := b.deps.Catalog.T(gl, i18n.LockdownNotice, nil)
	if strings.TrimSpace(reason) != "" {
		text += "\n" + b.deps.Catalog.T(gl, i18n.LockdownNoticeReason, map[string]any{"Reason": escapeMarkdown(reason)})
	}
	return text
}

func (b *Bot) handleLockdownServer(data discord.SlashCommandInteractionData, e *handler.CommandEvent) error {
	mc, ok := b.modCtx(e)
	if !ok {
		return errNotInGuild
	}
	if on, err := b.guardianOn(e, mc); !on || err != nil {
		return err
	}
	l := discord.Locale(mc.locale)
	reason, _ := data.OptString("reason")
	if err := e.DeferCreateMessage(true); err != nil {
		return err
	}
	var targets []snowflake.ID
	for ch := range b.client.Caches.ChannelsForGuild(mc.guildID) {
		if ow, _, ok := b.lockable(mc.guildID, ch.ID()); ok && !ow.Deny.Has(discord.PermissionSendMessages) {
			targets = append(targets, ch.ID())
		}
		if len(targets) == maxLockChannels {
			break
		}
	}
	// The notice goes out first: after the lock Niro may not be able to send there.
	b.postNotice(e.Channel().ID(), b.lockNotice(mc.guildID, reason))
	locked, failed := b.forEachChannel(targets, func(id snowflake.ID) (bool, error) {
		return b.lockChannel(e.Ctx, mc.guildID, id, mc.actor.ID, auditText(mc.actor.Name, reason))
	})
	text := b.t(l, i18n.LockdownDoneServer, map[string]any{"Count": locked})
	if failed > 0 {
		text += "\n" + b.t(l, i18n.LockdownFailed, map[string]any{"Count": failed})
	}
	return replyDeferred(e.UpdateInteractionResponse, b.infoCard(mc.locale, EmojiOK, text, barPass))
}

// forEachChannel runs fn on channels with a little parallelism (each channel
// has its own rate limit) and counts successes and failures.
func (b *Bot) forEachChannel(ids []snowflake.ID, fn func(snowflake.ID) (bool, error)) (done, failed int) {
	var (
		wg  sync.WaitGroup
		mu  sync.Mutex
		sem = make(chan struct{}, 4)
	)
	for _, id := range ids {
		wg.Go(func() {
			sem <- struct{}{}
			defer func() { <-sem }()
			ok, err := fn(id)
			mu.Lock()
			defer mu.Unlock()
			switch {
			case err != nil:
				failed++
				b.log.Warn("channel lock change failed", "channel_id", id, "err", err)
			case ok:
				done++
			}
		})
	}
	wg.Wait()
	return done, failed
}

func (b *Bot) handleUnlockChannel(data discord.SlashCommandInteractionData, e *handler.CommandEvent) error {
	mc, ok := b.modCtx(e)
	if !ok {
		return errNotInGuild
	}
	l := discord.Locale(mc.locale)
	chID := e.Channel().ID()
	if c, ok := data.OptChannel("channel"); ok {
		chID = c.ID
	}
	mention := "<#" + chID.String() + ">"
	locks, err := b.guardianStore().Lockdowns(e.Ctx, mc.guildID)
	if err != nil {
		return err
	}
	for _, st := range locks {
		if st.ChannelID != chID {
			continue
		}
		if err := b.unlockChannel(e.Ctx, st, mc.actor.Name+": unlock"); err != nil {
			b.log.Warn("unlock failed", "channel_id", chID, "err", err)
			return b.replyNow(e, b.infoCard(mc.locale, EmojiError, b.t(l, i18n.LockdownFailed, map[string]any{"Count": 1}), barFailed))
		}
		b.postNotice(chID, b.deps.Catalog.T(b.guildLocale(mc.guildID), i18n.LockdownUnlockedNotice, nil))
		return b.replyNow(e, b.infoCard(mc.locale, EmojiOK, b.t(l, i18n.LockdownUnlockedChannel, map[string]any{"Channel": mention}), barPass))
	}
	return b.replyNow(e, b.infoCard(mc.locale, EmojiInfo, b.t(l, i18n.LockdownNotLocked, map[string]any{"Channel": mention}), barNone))
}

func (b *Bot) handleUnlockServer(_ discord.SlashCommandInteractionData, e *handler.CommandEvent) error {
	mc, ok := b.modCtx(e)
	if !ok {
		return errNotInGuild
	}
	l := discord.Locale(mc.locale)
	if err := e.DeferCreateMessage(true); err != nil {
		return err
	}
	locks, err := b.guardianStore().Lockdowns(e.Ctx, mc.guildID)
	if err != nil {
		return err
	}
	if len(locks) == 0 {
		return replyDeferred(e.UpdateInteractionResponse, b.infoCard(mc.locale, EmojiInfo, b.t(l, i18n.LockdownNone, nil), barNone))
	}
	byID := map[snowflake.ID]guardian.LockState{}
	ids := make([]snowflake.ID, 0, len(locks))
	for _, st := range locks {
		byID[st.ChannelID] = st
		ids = append(ids, st.ChannelID)
	}
	unlocked, failed := b.forEachChannel(ids, func(id snowflake.ID) (bool, error) {
		return true, b.unlockChannel(e.Ctx, byID[id], mc.actor.Name+": unlock")
	})
	b.postNotice(e.Channel().ID(), b.deps.Catalog.T(b.guildLocale(mc.guildID), i18n.LockdownUnlockedNotice, nil))
	text := b.t(l, i18n.LockdownUnlockedServer, map[string]any{"Count": unlocked})
	if failed > 0 {
		text += "\n" + b.t(l, i18n.LockdownFailed, map[string]any{"Count": failed})
	}
	return replyDeferred(e.UpdateInteractionResponse, b.infoCard(mc.locale, EmojiOK, text, barPass))
}
