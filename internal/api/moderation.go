package api

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"time"

	"github.com/disgoorg/snowflake/v2"

	"github.com/nithron/niro/internal/api/apigen"
	"github.com/nithron/niro/internal/core"
	"github.com/nithron/niro/internal/modules/moderation"
	dc "github.com/nithron/niro/internal/platform/discord"
	"github.com/nithron/niro/internal/rpc"
)

// RPCCaller asks the bot to do things that need the bot token.
type RPCCaller interface {
	Call(ctx context.Context, method string, guildID, actorID snowflake.ID, params, out any) error
}

// apiError is returned by handlers for non-success outcomes; the strict
// server's response error handler renders it (see New).
type apiError struct {
	status int
	code   apigen.ErrorCode
	msg    string
}

func (e *apiError) Error() string { return fmt.Sprintf("%d %s: %s", e.status, e.code, e.msg) }

func httpErr(status int, code apigen.ErrorCode, msg string) error {
	return &apiError{status: status, code: code, msg: msg}
}

// guildErr maps authorization failures to HTTP errors.
func guildErr(err error) error {
	switch {
	case errors.Is(err, errForbidden):
		return httpErr(http.StatusForbidden, apigen.ErrorCodeForbidden, "You can't manage this server.")
	case errors.Is(err, errBotNotInGuild):
		return httpErr(http.StatusConflict, apigen.ErrorCodeBotNotInGuild, "Add Niro to this server first.")
	}
	return err
}

// rpcErr maps bot-side failures to HTTP errors.
func rpcErr(err error) error {
	var re *rpc.Error
	if !errors.As(err, &re) {
		return err
	}
	switch re.Code {
	case rpc.CodeInvalid:
		return httpErr(http.StatusBadRequest, apigen.ErrorCodeBadRequest, re.Message)
	case rpc.CodeNotFound:
		return httpErr(http.StatusNotFound, apigen.ErrorCodeNotFound, "Not found.")
	case rpc.CodeDiscord:
		return httpErr(http.StatusBadGateway, apigen.ErrorCodeDiscordError, "Discord: "+re.Message)
	case rpc.CodeUnavailable:
		return httpErr(http.StatusBadGateway, apigen.ErrorCodeBotUnavailable, "The bot is not responding right now. Please try again in a moment.")
	}
	return httpErr(http.StatusBadGateway, apigen.ErrorCodeInternal, "The bot could not complete the request.")
}

// managed authorizes a guild route that needs the bot present and returns the guild ID.
func (h handlers) managed(ctx context.Context, rawID string) (snowflake.ID, error) {
	g, _, err := h.authorizeGuild(ctx, rawID, true)
	if err != nil {
		return 0, guildErr(err)
	}
	return g.ID, nil
}

func actorID(ctx context.Context) snowflake.ID { return sessionFrom(ctx).User.ID }

// convert copies between API and domain types that share a JSON shape.
func convert(dst, src any) error {
	b, err := json.Marshal(src)
	if err != nil {
		return err
	}
	return json.Unmarshal(b, dst)
}

// ---- guild meta -------------------------------------------------------------

const guildMetaTTL = 30 * time.Second

func (h handlers) guildMeta(ctx context.Context, guildID snowflake.ID) (dc.GuildMeta, error) {
	key := "niro:guild-meta:" + guildID.String()
	var meta dc.GuildMeta
	if found, err := h.kv.GetJSON(ctx, key, &meta); err == nil && found {
		return meta, nil
	}
	if err := h.rpc.Call(ctx, dc.RPCGuildMeta, guildID, actorID(ctx), nil, &meta); err != nil {
		return meta, rpcErr(err)
	}
	if err := h.kv.SetJSON(ctx, key, meta, guildMetaTTL); err != nil {
		h.log.WarnContext(ctx, "caching guild meta failed", "err", err)
	}
	return meta, nil
}

func (h handlers) GetGuildMeta(ctx context.Context, req apigen.GetGuildMetaRequestObject) (apigen.GetGuildMetaResponseObject, error) {
	gid, err := h.managed(ctx, req.GuildId)
	if err != nil {
		return nil, err
	}
	meta, err := h.guildMeta(ctx, gid)
	if err != nil {
		return nil, err
	}
	var out apigen.GuildMeta
	if err := convert(&out, meta); err != nil {
		return nil, err
	}
	return apigen.GetGuildMeta200JSONResponse(out), nil
}

// ---- settings ---------------------------------------------------------------

func toAPISettings(s moderation.Settings) apigen.ModerationSettings {
	return apigen.ModerationSettings{LogChannelId: s.LogChannelID.String(), DmUsers: s.DMUsers, RequireReason: s.RequireReason}
}

func (h handlers) GetModerationSettings(ctx context.Context, req apigen.GetModerationSettingsRequestObject) (apigen.GetModerationSettingsResponseObject, error) {
	gid, err := h.managed(ctx, req.GuildId)
	if err != nil {
		return nil, err
	}
	s, err := moderation.LoadSettings(ctx, h.config, gid)
	if err != nil {
		return nil, err
	}
	return apigen.GetModerationSettings200JSONResponse(toAPISettings(s)), nil
}

func (h handlers) UpdateModerationSettings(ctx context.Context, req apigen.UpdateModerationSettingsRequestObject) (apigen.UpdateModerationSettingsResponseObject, error) {
	gid, err := h.managed(ctx, req.GuildId)
	if err != nil {
		return nil, err
	}
	if req.Body == nil {
		return nil, httpErr(http.StatusBadRequest, apigen.ErrorCodeBadRequest, "Missing request body.")
	}
	logChannel, err := snowflake.Parse(req.Body.LogChannelId)
	if err != nil {
		return nil, httpErr(http.StatusBadRequest, apigen.ErrorCodeBadRequest, "Invalid log channel.")
	}
	if logChannel != 0 {
		meta, err := h.guildMeta(ctx, gid)
		if err != nil {
			return nil, err
		}
		if !hasTextChannel(meta, logChannel) {
			return nil, httpErr(http.StatusBadRequest, apigen.ErrorCodeBadRequest, "The log channel must be a text or announcement channel in this server.")
		}
	}
	raw, _ := json.Marshal(moderation.Settings{LogChannelID: logChannel, DMUsers: req.Body.DmUsers, RequireReason: req.Body.RequireReason})
	actor := actorID(ctx)
	saved, err := h.config.SetSettings(ctx, gid, core.ModuleModeration, raw, &actor, core.SourceDashboard)
	if errors.Is(err, core.ErrInvalidSettings) {
		return nil, httpErr(http.StatusBadRequest, apigen.ErrorCodeBadRequest, err.Error())
	}
	if err != nil {
		return nil, err
	}
	var s moderation.Settings
	if err := json.Unmarshal(saved, &s); err != nil {
		return nil, err
	}
	return apigen.UpdateModerationSettings200JSONResponse(toAPISettings(s)), nil
}

func hasTextChannel(meta dc.GuildMeta, id snowflake.ID) bool {
	for _, c := range meta.Channels {
		if c.ID == id && (c.Type == "text" || c.Type == "announcement") {
			return true
		}
	}
	return false
}

// ---- cases ------------------------------------------------------------------

func toAPICase(c moderation.Case) apigen.Case {
	out := apigen.Case{
		Number: c.Number, Action: apigen.CaseAction(c.Action), TargetName: c.TargetName,
		ModeratorId: c.ModeratorID.String(), ModeratorName: c.ModeratorName, Reason: c.Reason,
		Source: apigen.CaseSource(c.Source), DmSent: c.DMSent, CreatedAt: c.CreatedAt, UpdatedAt: c.UpdatedAt,
		ExpiresAt: c.ExpiresAt,
	}
	if c.TargetID != 0 {
		id := c.TargetID.String()
		out.TargetId = &id
	}
	if c.Duration > 0 {
		s := int(c.Duration.Seconds())
		out.DurationSeconds = &s
	}
	if c.Details.ChannelID != 0 {
		id := c.Details.ChannelID.String()
		out.ChannelId = &id
	}
	switch {
	case c.Action == moderation.ActionPurge:
		n := c.Details.Deleted
		out.DeletedMessages = &n
	case c.Details.DeleteMessages > 0:
		n := c.Details.DeleteMessages
		out.DeletedMessages = &n
	}
	if c.Details.Temporary {
		t := true
		out.Temporary = &t
	}
	return out
}

func optSnowflake(s *string) (snowflake.ID, error) {
	if s == nil || *s == "" {
		return 0, nil
	}
	return snowflake.Parse(*s)
}

func (h handlers) ListCases(ctx context.Context, req apigen.ListCasesRequestObject) (apigen.ListCasesResponseObject, error) {
	gid, err := h.managed(ctx, req.GuildId)
	if err != nil {
		return nil, err
	}
	f := moderation.CaseFilter{Limit: 25}
	p := req.Params
	if f.TargetID, err = optSnowflake(p.User); err != nil {
		return nil, httpErr(http.StatusBadRequest, apigen.ErrorCodeBadRequest, "Invalid user ID.")
	}
	if f.ModeratorID, err = optSnowflake(p.Moderator); err != nil {
		return nil, httpErr(http.StatusBadRequest, apigen.ErrorCodeBadRequest, "Invalid moderator ID.")
	}
	if p.Action != nil {
		f.Action = moderation.Action(*p.Action)
		if !f.Action.Valid() {
			return nil, httpErr(http.StatusBadRequest, apigen.ErrorCodeBadRequest, "Unknown action.")
		}
	}
	if p.Before != nil {
		f.Before = *p.Before
	}
	if p.Limit != nil && *p.Limit >= 1 && *p.Limit <= 100 {
		f.Limit = *p.Limit
	}
	cases, err := h.cases.ListCases(ctx, gid, f)
	if err != nil {
		return nil, err
	}
	total, err := h.cases.CountCases(ctx, gid, f)
	if err != nil {
		return nil, err
	}
	out := apigen.CaseList{Items: make([]apigen.Case, len(cases)), Total: total}
	for i, c := range cases {
		out.Items[i] = toAPICase(c)
	}
	if len(cases) == f.Limit && cases[len(cases)-1].Number > 1 {
		next := cases[len(cases)-1].Number
		out.NextBefore = &next
	}
	return apigen.ListCases200JSONResponse(out), nil
}

func (h handlers) getCase(ctx context.Context, gid snowflake.ID, n int) (moderation.Case, error) {
	c, err := h.cases.GetCase(ctx, gid, n)
	if errors.Is(err, moderation.ErrCaseNotFound) {
		return c, httpErr(http.StatusNotFound, apigen.ErrorCodeNotFound, fmt.Sprintf("Case #%d doesn't exist.", n))
	}
	return c, err
}

func (h handlers) GetCase(ctx context.Context, req apigen.GetCaseRequestObject) (apigen.GetCaseResponseObject, error) {
	gid, err := h.managed(ctx, req.GuildId)
	if err != nil {
		return nil, err
	}
	c, err := h.getCase(ctx, gid, req.CaseNumber)
	if err != nil {
		return nil, err
	}
	return apigen.GetCase200JSONResponse(toAPICase(c)), nil
}

func (h handlers) UpdateCase(ctx context.Context, req apigen.UpdateCaseRequestObject) (apigen.UpdateCaseResponseObject, error) {
	gid, err := h.managed(ctx, req.GuildId)
	if err != nil {
		return nil, err
	}
	if req.Body == nil {
		return nil, httpErr(http.StatusBadRequest, apigen.ErrorCodeBadRequest, "Missing request body.")
	}
	if _, err := h.getCase(ctx, gid, req.CaseNumber); err != nil {
		return nil, err
	}
	// The bot updates the case and its log message.
	if err := h.rpc.Call(ctx, dc.RPCCaseReason, gid, actorID(ctx), dc.CaseReasonParams{Number: req.CaseNumber, Reason: req.Body.Reason}, nil); err != nil {
		return nil, rpcErr(err)
	}
	c, err := h.getCase(ctx, gid, req.CaseNumber)
	if err != nil {
		return nil, err
	}
	return apigen.UpdateCase200JSONResponse(toAPICase(c)), nil
}

func (h handlers) DeleteCase(ctx context.Context, req apigen.DeleteCaseRequestObject) (apigen.DeleteCaseResponseObject, error) {
	gid, err := h.managed(ctx, req.GuildId)
	if err != nil {
		return nil, err
	}
	if _, err := h.getCase(ctx, gid, req.CaseNumber); err != nil {
		return nil, err
	}
	if err := h.rpc.Call(ctx, dc.RPCCaseDelete, gid, actorID(ctx), dc.CaseNumberParams{Number: req.CaseNumber}, nil); err != nil {
		return nil, rpcErr(err)
	}
	return apigen.DeleteCase204Response{}, nil
}

// ---- AutoMod ----------------------------------------------------------------

func (h handlers) ListAutoModRules(ctx context.Context, req apigen.ListAutoModRulesRequestObject) (apigen.ListAutoModRulesResponseObject, error) {
	gid, err := h.managed(ctx, req.GuildId)
	if err != nil {
		return nil, err
	}
	var rules []moderation.AutoModRule
	if err := h.rpc.Call(ctx, dc.RPCAutoModList, gid, actorID(ctx), nil, &rules); err != nil {
		return nil, rpcErr(err)
	}
	out := apigen.ListAutoModRules200JSONResponse{}
	if err := convert(&out, rules); err != nil {
		return nil, err
	}
	return out, nil
}

// ruleFromBody validates a submitted rule before it goes to the bot, so
// errors come back fast and in the same words the bot would use.
func ruleFromBody(body *apigen.AutoModRule) (moderation.AutoModRule, error) {
	var rule moderation.AutoModRule
	if body == nil {
		return rule, httpErr(http.StatusBadRequest, apigen.ErrorCodeBadRequest, "Missing request body.")
	}
	if err := convert(&rule, body); err != nil {
		return rule, httpErr(http.StatusBadRequest, apigen.ErrorCodeBadRequest, "Invalid rule.")
	}
	if err := rule.Normalize(); err != nil {
		return rule, httpErr(http.StatusBadRequest, apigen.ErrorCodeBadRequest, err.Error())
	}
	return rule, nil
}

func (h handlers) CreateAutoModRule(ctx context.Context, req apigen.CreateAutoModRuleRequestObject) (apigen.CreateAutoModRuleResponseObject, error) {
	gid, err := h.managed(ctx, req.GuildId)
	if err != nil {
		return nil, err
	}
	rule, err := ruleFromBody(req.Body)
	if err != nil {
		return nil, err
	}
	rule.ID = 0
	var created moderation.AutoModRule
	if err := h.rpc.Call(ctx, dc.RPCAutoModCreate, gid, actorID(ctx), rule, &created); err != nil {
		return nil, rpcErr(err)
	}
	h.log.InfoContext(ctx, "automod rule created from dashboard", "guild_id", gid, "rule_id", created.ID, "user_id", actorID(ctx))
	var out apigen.CreateAutoModRule201JSONResponse
	if err := convert(&out, created); err != nil {
		return nil, err
	}
	return out, nil
}

func (h handlers) UpdateAutoModRule(ctx context.Context, req apigen.UpdateAutoModRuleRequestObject) (apigen.UpdateAutoModRuleResponseObject, error) {
	gid, err := h.managed(ctx, req.GuildId)
	if err != nil {
		return nil, err
	}
	rule, err := ruleFromBody(req.Body)
	if err != nil {
		return nil, err
	}
	if rule.ID, err = snowflake.Parse(req.RuleId); err != nil {
		return nil, httpErr(http.StatusBadRequest, apigen.ErrorCodeBadRequest, "Invalid rule ID.")
	}
	var updated moderation.AutoModRule
	if err := h.rpc.Call(ctx, dc.RPCAutoModUpdate, gid, actorID(ctx), rule, &updated); err != nil {
		return nil, rpcErr(err)
	}
	var out apigen.UpdateAutoModRule200JSONResponse
	if err := convert(&out, updated); err != nil {
		return nil, err
	}
	return out, nil
}

func (h handlers) DeleteAutoModRule(ctx context.Context, req apigen.DeleteAutoModRuleRequestObject) (apigen.DeleteAutoModRuleResponseObject, error) {
	gid, err := h.managed(ctx, req.GuildId)
	if err != nil {
		return nil, err
	}
	id, err := snowflake.Parse(req.RuleId)
	if err != nil {
		return nil, httpErr(http.StatusBadRequest, apigen.ErrorCodeBadRequest, "Invalid rule ID.")
	}
	if err := h.rpc.Call(ctx, dc.RPCAutoModDelete, gid, actorID(ctx), dc.AutoModIDParams{ID: id}, nil); err != nil {
		return nil, rpcErr(err)
	}
	return apigen.DeleteAutoModRule204Response{}, nil
}

func (h handlers) ListAutoModPresets(_ context.Context, _ apigen.ListAutoModPresetsRequestObject) (apigen.ListAutoModPresetsResponseObject, error) {
	out := apigen.ListAutoModPresets200JSONResponse{}
	if err := convert(&out, moderation.AutoModPresets()); err != nil {
		return nil, err
	}
	return out, nil
}
