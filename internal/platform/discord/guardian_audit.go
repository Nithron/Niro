package discord

import (
	"context"
	"errors"
	"net/http"
	"slices"
	"strings"
	"sync"

	"github.com/disgoorg/disgo/discord"
	"github.com/disgoorg/disgo/rest"
	"github.com/disgoorg/snowflake/v2"

	"github.com/nithron/niro/internal/modules/guardian"
)

// permissionNames maps Discord permission bits to the audit's names.
var auditPermissions = []struct {
	bit  discord.Permissions
	name string
}{
	{discord.PermissionAdministrator, guardian.PermAdministrator},
	{discord.PermissionManageGuild, guardian.PermManageGuild},
	{discord.PermissionManageRoles, guardian.PermManageRoles},
	{discord.PermissionManageChannels, guardian.PermManageChannels},
	{discord.PermissionManageWebhooks, guardian.PermManageWebhooks},
	{discord.PermissionBanMembers, guardian.PermBanMembers},
	{discord.PermissionKickMembers, guardian.PermKickMembers},
	{discord.PermissionModerateMembers, guardian.PermModerateMembers},
	{discord.PermissionManageMessages, guardian.PermManageMessages},
	{discord.PermissionMentionEveryone, guardian.PermMentionEveryone},
	{discord.PermissionManageGuildExpressions, guardian.PermManageExprs},
}

func dangerousNames(p discord.Permissions) []string {
	var out []string
	for _, a := range auditPermissions {
		if p.Has(a.bit) {
			out = append(out, a.name)
		}
	}
	return out
}

// AuditResult is the audit plus whether webhooks could be listed.
type AuditResult struct {
	Findings        []guardian.Finding
	WebhooksChecked bool
}

// runAudit reads the server's roles, bots, webhooks and safety settings and
// runs the Guardian audit. It only reads; nothing is changed.
func (b *Bot) runAudit(ctx context.Context, guildID snowflake.ID) (AuditResult, error) {
	in := guardian.AuditInput{}
	if g, ok := b.client.Caches.Guild(guildID); ok {
		in.VerificationLevel = int(g.VerificationLevel)
		in.ModeratorMFA = g.MFALevel == discord.MFALevelElevated
	} else {
		g, err := b.client.Rest.GetGuild(guildID, false, rest.WithCtx(ctx))
		if err != nil {
			return AuditResult{}, err
		}
		in.VerificationLevel = int(g.VerificationLevel)
		in.ModeratorMFA = g.MFALevel == discord.MFALevelElevated
	}

	var (
		wg         sync.WaitGroup
		counts     map[snowflake.ID]int
		webhooks   []discord.Webhook
		webhookErr error
	)
	wg.Go(func() { counts, _ = b.client.Rest.GetRoleMemberCounts(guildID, rest.WithCtx(ctx)) })
	wg.Go(func() { webhooks, webhookErr = b.client.Rest.GetAllWebhooks(guildID, rest.WithCtx(ctx)) })
	wg.Wait()

	for r := range b.client.Caches.Roles(guildID) {
		if r.ID == guildID {
			in.EveryonePermissions = dangerousNames(r.Permissions)
			continue
		}
		n := -1
		if c, ok := counts[r.ID]; ok {
			n = c
		}
		in.Roles = append(in.Roles, guardian.AuditRole{
			ID: r.ID, Name: r.Name, Position: r.Position, Permissions: dangerousNames(r.Permissions), Managed: r.Managed, Members: n,
		})
	}
	slices.SortFunc(in.Roles, func(a, b guardian.AuditRole) int { return b.Position - a.Position })

	// Bots: every bot gets a managed role tagged with its ID; cached bot
	// members cover bots without one. Their effective permissions are then
	// computed by Discord.
	self, _ := b.client.Caches.SelfUser()
	botIDs := map[snowflake.ID]string{self.ID: self.Username}
	for r := range b.client.Caches.Roles(guildID) {
		if r.Tags != nil && r.Tags.BotID != nil {
			if _, ok := botIDs[*r.Tags.BotID]; !ok {
				botIDs[*r.Tags.BotID] = r.Name
			}
		}
	}
	for m := range b.client.Caches.Members(guildID) {
		if m.User.Bot {
			botIDs[m.User.ID] = m.User.Username
		}
	}
	var mu sync.Mutex
	sem := make(chan struct{}, 4)
	for id, name := range botIDs {
		wg.Go(func() {
			sem <- struct{}{}
			defer func() { <-sem }()
			var perms discord.Permissions
			if m, ok := b.client.Caches.Member(guildID, id); ok {
				perms = b.client.Caches.MemberPermissions(m)
			} else if p, err := b.client.Rest.GetGuildMemberPermissions(guildID, id, rest.WithCtx(ctx)); err == nil {
				perms = p
			} else {
				return // left the server
			}
			mu.Lock()
			in.Bots = append(in.Bots, guardian.AuditBot{ID: id, Name: name, Permissions: dangerousNames(perms), Self: id == self.ID})
			mu.Unlock()
		})
	}
	wg.Wait()
	slices.SortFunc(in.Bots, func(a, b guardian.AuditBot) int { return strings.Compare(a.Name, b.Name) })

	res := AuditResult{WebhooksChecked: webhookErr == nil}
	var re *rest.Error
	if webhookErr != nil && !(asRestError(webhookErr, &re) && re.Response != nil && re.Response.StatusCode == http.StatusForbidden) {
		b.log.WarnContext(ctx, "listing webhooks failed", "guild_id", guildID, "err", webhookErr)
	}
	for _, w := range webhooks {
		if iw, ok := w.(discord.IncomingWebhook); ok {
			aw := guardian.AuditWebhook{ID: iw.ID(), Name: iw.Name(), ChannelID: iw.ChannelID}
			if iw.User != nil {
				aw.CreatorID, aw.Creator = iw.User.ID, iw.User.Username
			}
			in.Webhooks = append(in.Webhooks, aw)
		}
	}
	res.Findings = guardian.Audit(in)
	return res, nil
}

func asRestError(err error, target **rest.Error) bool { return errors.As(err, target) }
