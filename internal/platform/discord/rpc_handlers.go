package discord

import (
	"context"
	"errors"
	"slices"

	"github.com/disgoorg/disgo/discord"
	"github.com/disgoorg/disgo/rest"
	"github.com/disgoorg/snowflake/v2"

	"github.com/nithron/niro/internal/modules/moderation"
	"github.com/nithron/niro/internal/rpc"
)

// RPC method names (shared with the API).
const (
	RPCGuildMeta     = "guild.meta"
	RPCCaseReason    = "case.reason"
	RPCCaseDelete    = "case.delete"
	RPCAutoModList   = "automod.list"
	RPCAutoModCreate = "automod.create"
	RPCAutoModUpdate = "automod.update"
	RPCAutoModDelete = "automod.delete"
)

// GuildMeta is what the dashboard needs to show channel and role pickers.
type GuildMeta struct {
	Channels []MetaChannel `json:"channels"`
	Roles    []MetaRole    `json:"roles"`
}

type MetaChannel struct {
	ID       snowflake.ID `json:"id"`
	Name     string       `json:"name"`
	Type     string       `json:"type"` // text | announcement | voice | forum | category | other
	ParentID snowflake.ID `json:"parentId,omitempty"`
	Position int          `json:"position"`
}

type MetaRole struct {
	ID       snowflake.ID `json:"id"`
	Name     string       `json:"name"`
	Color    int          `json:"color"`
	Position int          `json:"position"`
	Managed  bool         `json:"managed"`
}

type CaseReasonParams struct {
	Number int    `json:"number"`
	Reason string `json:"reason"`
}

type CaseNumberParams struct {
	Number int `json:"number"`
}

type AutoModIDParams struct {
	ID snowflake.ID `json:"id"`
}

// RegisterRPC adds the bot's RPC handlers to srv.
func (b *Bot) RegisterRPC(srv *rpc.Server) {
	srv.Handle(RPCGuildMeta, b.rpcGuildMeta)
	srv.Handle(RPCCaseReason, func(ctx context.Context, r rpc.Request) (any, error) {
		var p CaseReasonParams
		if err := r.Bind(&p); err != nil {
			return nil, err
		}
		_, err := b.mod.UpdateReason(ctx, r.GuildID, p.Number, p.Reason)
		return nil, caseRPCError(err)
	})
	srv.Handle(RPCCaseDelete, func(ctx context.Context, r rpc.Request) (any, error) {
		var p CaseNumberParams
		if err := r.Bind(&p); err != nil {
			return nil, err
		}
		_, err := b.mod.DeleteCase(ctx, r.GuildID, p.Number, r.ActorID)
		return nil, caseRPCError(err)
	})
	srv.Handle(RPCAutoModList, b.rpcAutoModList)
	srv.Handle(RPCAutoModCreate, b.rpcAutoModCreate)
	srv.Handle(RPCAutoModUpdate, b.rpcAutoModUpdate)
	srv.Handle(RPCAutoModDelete, b.rpcAutoModDelete)
}

func caseRPCError(err error) error {
	switch {
	case err == nil:
		return nil
	case errors.Is(err, moderation.ErrCaseNotFound):
		return rpc.Errorf(rpc.CodeNotFound, "case not found")
	case errors.Is(err, moderation.ErrReasonTooLong):
		return rpc.Errorf(rpc.CodeInvalid, "reasons can be at most %d characters", moderation.MaxReasonLength)
	}
	return err
}

// discordRPCError passes Discord's own message through (e.g. "Maximum number
// of rules of this trigger type reached"), which is useful to the user.
func discordRPCError(err error) error {
	var re *rest.Error
	if errors.As(err, &re) {
		msg := re.Message
		if msg == "" {
			msg = "Discord rejected the request"
		}
		return rpc.Errorf(rpc.CodeDiscord, "%s", msg)
	}
	return err
}

func (b *Bot) rpcGuildMeta(ctx context.Context, r rpc.Request) (any, error) {
	chans, err := b.client.Rest.GetGuildChannels(r.GuildID, rest.WithCtx(ctx))
	if err != nil {
		return nil, discordRPCError(err)
	}
	roles, err := b.client.Rest.GetRoles(r.GuildID, rest.WithCtx(ctx))
	if err != nil {
		return nil, discordRPCError(err)
	}
	meta := GuildMeta{Channels: []MetaChannel{}, Roles: []MetaRole{}}
	for _, c := range chans {
		mc := MetaChannel{ID: c.ID(), Name: c.Name(), Type: channelKind(c.Type()), Position: c.Position()}
		if p := c.ParentID(); p != nil {
			mc.ParentID = *p
		}
		meta.Channels = append(meta.Channels, mc)
	}
	for _, role := range roles {
		if role.ID == r.GuildID {
			continue // @everyone
		}
		meta.Roles = append(meta.Roles, MetaRole{ID: role.ID, Name: role.Name, Color: role.Color, Position: role.Position, Managed: role.Managed})
	}
	slices.SortFunc(meta.Roles, func(a, b MetaRole) int { return b.Position - a.Position })
	slices.SortFunc(meta.Channels, func(a, b MetaChannel) int { return a.Position - b.Position })
	return meta, nil
}

func channelKind(t discord.ChannelType) string {
	switch t {
	case discord.ChannelTypeGuildText:
		return "text"
	case discord.ChannelTypeGuildNews:
		return "announcement"
	case discord.ChannelTypeGuildVoice, discord.ChannelTypeGuildStageVoice:
		return "voice"
	case discord.ChannelTypeGuildForum, discord.ChannelTypeGuildMedia:
		return "forum"
	case discord.ChannelTypeGuildCategory:
		return "category"
	}
	return "other"
}

// ---- AutoMod --------------------------------------------------------------

var presetNames = map[string]discord.AutoModerationKeywordPreset{
	"profanity":      discord.AutoModerationKeywordPresetProfanity,
	"sexual_content": discord.AutoModerationKeywordPresetSexualContent,
	"slurs":          discord.AutoModerationKeywordPresetSlurs,
}

var triggerTypes = map[string]discord.AutoModerationTriggerType{
	moderation.TriggerKeyword:       discord.AutoModerationTriggerTypeKeyword,
	moderation.TriggerSpam:          discord.AutoModerationTriggerTypeSpam,
	moderation.TriggerKeywordPreset: discord.AutoModerationTriggerTypeKeywordPresent,
	moderation.TriggerMentionSpam:   discord.AutoModerationTriggerTypeMentionSpam,
}

func fromDiscordRule(r discord.AutoModerationRule, botID snowflake.ID) (moderation.AutoModRule, bool) {
	out := moderation.AutoModRule{
		ID: r.ID, Name: r.Name, Enabled: r.Enabled, CreatedByBot: r.CreatorID == botID,
		Keywords: r.TriggerMetadata.KeywordFilter, RegexPatterns: r.TriggerMetadata.RegexPatterns,
		AllowList: r.TriggerMetadata.AllowList, MentionLimit: r.TriggerMetadata.MentionTotalLimit,
		MentionRaidProtection: r.TriggerMetadata.MentionRaidProtectionEnabled,
		ExemptRoleIDs:         r.ExemptRoles, ExemptChannelIDs: r.ExemptChannels,
	}
	found := false
	for name, t := range triggerTypes {
		if t == r.TriggerType {
			out.Trigger, found = name, true
		}
	}
	if !found {
		return out, false // e.g. member-profile rules: not managed by the dashboard
	}
	for _, p := range r.TriggerMetadata.Presets {
		for name, v := range presetNames {
			if v == p {
				out.Presets = append(out.Presets, name)
			}
		}
	}
	for _, a := range r.Actions {
		switch a.Type {
		case discord.AutoModerationActionTypeBlockMessage:
			out.BlockMessage = true
			if a.Metadata != nil && a.Metadata.CustomMessage != nil {
				out.CustomMessage = *a.Metadata.CustomMessage
			}
		case discord.AutoModerationActionTypeSendAlertMessage:
			if a.Metadata != nil {
				out.AlertChannelID = a.Metadata.ChannelID
			}
		case discord.AutoModerationActionTypeTimeout:
			if a.Metadata != nil {
				out.TimeoutSeconds = a.Metadata.DurationSeconds
			}
		}
	}
	return out, true
}

func toDiscordParts(r moderation.AutoModRule) (*discord.AutoModerationTriggerMetadata, []discord.AutoModerationAction) {
	var meta *discord.AutoModerationTriggerMetadata
	switch r.Trigger {
	case moderation.TriggerKeyword:
		meta = &discord.AutoModerationTriggerMetadata{KeywordFilter: r.Keywords, RegexPatterns: r.RegexPatterns, AllowList: r.AllowList}
	case moderation.TriggerKeywordPreset:
		meta = &discord.AutoModerationTriggerMetadata{AllowList: r.AllowList}
		for _, p := range r.Presets {
			meta.Presets = append(meta.Presets, presetNames[p])
		}
	case moderation.TriggerMentionSpam:
		meta = &discord.AutoModerationTriggerMetadata{MentionTotalLimit: r.MentionLimit, MentionRaidProtectionEnabled: r.MentionRaidProtection}
	}
	var actions []discord.AutoModerationAction
	if r.BlockMessage {
		a := discord.AutoModerationAction{Type: discord.AutoModerationActionTypeBlockMessage}
		if r.CustomMessage != "" {
			msg := r.CustomMessage
			a.Metadata = &discord.AutoModerationActionMetadata{CustomMessage: &msg}
		}
		actions = append(actions, a)
	}
	if r.AlertChannelID != 0 {
		actions = append(actions, discord.AutoModerationAction{Type: discord.AutoModerationActionTypeSendAlertMessage,
			Metadata: &discord.AutoModerationActionMetadata{ChannelID: r.AlertChannelID}})
	}
	if r.TimeoutSeconds > 0 {
		actions = append(actions, discord.AutoModerationAction{Type: discord.AutoModerationActionTypeTimeout,
			Metadata: &discord.AutoModerationActionMetadata{DurationSeconds: r.TimeoutSeconds}})
	}
	return meta, actions
}

func (b *Bot) selfID() snowflake.ID {
	if u, ok := b.client.Caches.SelfUser(); ok {
		return u.ID
	}
	return 0
}

func (b *Bot) rpcAutoModList(ctx context.Context, r rpc.Request) (any, error) {
	rules, err := b.client.Rest.GetAutoModerationRules(r.GuildID, rest.WithCtx(ctx))
	if err != nil {
		return nil, discordRPCError(err)
	}
	out := []moderation.AutoModRule{}
	for _, rule := range rules {
		if m, ok := fromDiscordRule(rule, b.selfID()); ok {
			out = append(out, m)
		}
	}
	return out, nil
}

func bindRule(r rpc.Request) (moderation.AutoModRule, error) {
	var rule moderation.AutoModRule
	if err := r.Bind(&rule); err != nil {
		return rule, err
	}
	if err := rule.Normalize(); err != nil {
		return rule, rpc.Errorf(rpc.CodeInvalid, "%s", err.Error())
	}
	return rule, nil
}

func (b *Bot) rpcAutoModCreate(ctx context.Context, r rpc.Request) (any, error) {
	rule, err := bindRule(r)
	if err != nil {
		return nil, err
	}
	meta, actions := toDiscordParts(rule)
	created, err := b.client.Rest.CreateAutoModerationRule(r.GuildID, discord.AutoModerationRuleCreate{
		Name: rule.Name, EventType: discord.AutoModerationEventTypeMessageSend, TriggerType: triggerTypes[rule.Trigger],
		TriggerMetadata: meta, Actions: actions, Enabled: &rule.Enabled,
		ExemptRoles: rule.ExemptRoleIDs, ExemptChannels: rule.ExemptChannelIDs,
	}, rest.WithCtx(ctx), rest.WithReason("Created from the Niro dashboard"))
	if err != nil {
		return nil, discordRPCError(err)
	}
	out, _ := fromDiscordRule(*created, b.selfID())
	return out, nil
}

func (b *Bot) rpcAutoModUpdate(ctx context.Context, r rpc.Request) (any, error) {
	rule, err := bindRule(r)
	if err != nil {
		return nil, err
	}
	if rule.ID == 0 {
		return nil, rpc.Errorf(rpc.CodeInvalid, "missing rule id")
	}
	meta, actions := toDiscordParts(rule)
	exemptRoles, exemptChannels := nonNil(rule.ExemptRoleIDs), nonNil(rule.ExemptChannelIDs)
	updated, err := b.client.Rest.UpdateAutoModerationRule(r.GuildID, rule.ID, discord.AutoModerationRuleUpdate{
		Name: &rule.Name, TriggerMetadata: meta, Actions: &actions, Enabled: &rule.Enabled,
		ExemptRoles: &exemptRoles, ExemptChannels: &exemptChannels,
	}, rest.WithCtx(ctx), rest.WithReason("Updated from the Niro dashboard"))
	if isNotFound(err) {
		return nil, rpc.Errorf(rpc.CodeNotFound, "rule not found")
	}
	if err != nil {
		return nil, discordRPCError(err)
	}
	out, _ := fromDiscordRule(*updated, b.selfID())
	return out, nil
}

func (b *Bot) rpcAutoModDelete(ctx context.Context, r rpc.Request) (any, error) {
	var p AutoModIDParams
	if err := r.Bind(&p); err != nil {
		return nil, err
	}
	err := b.client.Rest.DeleteAutoModerationRule(r.GuildID, p.ID, rest.WithCtx(ctx), rest.WithReason("Deleted from the Niro dashboard"))
	if isNotFound(err) {
		return nil, rpc.Errorf(rpc.CodeNotFound, "rule not found")
	}
	return nil, discordRPCError(err)
}

func nonNil(ids []snowflake.ID) []snowflake.ID {
	if ids == nil {
		return []snowflake.ID{}
	}
	return ids
}

// isNotFound reports a 404 from Discord (used where disgo has no specific
// JSON error code constant).
func isNotFound(err error) bool {
	var re *rest.Error
	return errors.As(err, &re) && re.Response != nil && re.Response.StatusCode == 404
}
