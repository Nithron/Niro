package api

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/disgoorg/snowflake/v2"

	"github.com/nithron/niro/internal/api/apigen"
	"github.com/nithron/niro/internal/core"
	"github.com/nithron/niro/internal/modules/guardian"
	dc "github.com/nithron/niro/internal/platform/discord"
)

// GuardianReader is the read side of the Guardian store the API uses;
// every change goes through the bot (RPC) so Discord stays in sync.
type GuardianReader interface {
	GetEvent(ctx context.Context, guildID snowflake.ID, id int64) (guardian.Event, error)
	ListEvents(ctx context.Context, guildID snowflake.ID, f guardian.EventFilter) ([]guardian.Event, error)
	ActiveRaid(ctx context.Context, guildID snowflake.ID) (guardian.Raid, error)
}

// ---- settings ---------------------------------------------------------------

func ids(in []snowflake.ID) []apigen.Snowflake {
	out := make([]apigen.Snowflake, len(in))
	for i, id := range in {
		out[i] = id.String()
	}
	return out
}

func toAPIGuardianSettings(s guardian.Settings) apigen.GuardianSettings {
	nonNil := func(v []string) []string {
		if v == nil {
			return []string{}
		}
		return v
	}
	return apigen.GuardianSettings{
		Mode: apigen.GuardianSettingsMode(s.Mode), AlertChannelId: s.AlertChannelID.String(), ExemptRoleIds: ids(s.ExemptRoleIDs),
		BurstEnabled: s.BurstEnabled, BurstChannels: s.BurstChannels, BurstWindowSeconds: s.BurstWindowSeconds, QuietDays: s.QuietDays,
		ScamLinksEnabled: s.ScamLinksEnabled, AllowDomains: nonNil(s.AllowDomains), DenyDomains: nonNil(s.DenyDomains),
		QuarantineMode: apigen.GuardianSettingsQuarantineMode(s.QuarantineMode), QuarantineRoleId: s.QuarantineRoleID.String(),
		QuarantineHours: s.QuarantineHours, DmUser: s.DMUser,
		RaidEnabled: s.RaidEnabled, RaidJoins: s.RaidJoins, RaidWindowSeconds: s.RaidWindowSeconds,
		RaidDurationMinutes: s.RaidDurationMinutes, RaidPauseInvites: s.RaidPauseInvites,
		RaidRaiseVerification: s.RaidRaiseVerification, RaidSlowmodeSeconds: s.RaidSlowmodeSeconds,
		MinAccountAgeHours: s.MinAccountAgeHours, AccountAgeAction: apigen.GuardianSettingsAccountAgeAction(s.AccountAgeAction),
		VerifyRoleId: s.VerifyRoleID.String(), VerifyMinAccountAgeHours: s.VerifyMinAccountAgeHours,
	}
}

func fromAPIGuardianSettings(b apigen.GuardianSettings) (guardian.Settings, error) {
	bad := func(what string) error {
		return httpErr(http.StatusBadRequest, apigen.ErrorCodeBadRequest, "Invalid "+what+".")
	}
	s := guardian.Settings{
		Mode: guardian.Mode(b.Mode), BurstEnabled: b.BurstEnabled, BurstChannels: b.BurstChannels,
		BurstWindowSeconds: b.BurstWindowSeconds, QuietDays: b.QuietDays, ScamLinksEnabled: b.ScamLinksEnabled,
		AllowDomains: b.AllowDomains, DenyDomains: b.DenyDomains, QuarantineMode: guardian.QuarantineMode(b.QuarantineMode),
		QuarantineHours: b.QuarantineHours, DMUser: b.DmUser, RaidEnabled: b.RaidEnabled, RaidJoins: b.RaidJoins,
		RaidWindowSeconds: b.RaidWindowSeconds, RaidDurationMinutes: b.RaidDurationMinutes, RaidPauseInvites: b.RaidPauseInvites,
		RaidRaiseVerification: b.RaidRaiseVerification, RaidSlowmodeSeconds: b.RaidSlowmodeSeconds,
		MinAccountAgeHours: b.MinAccountAgeHours, AccountAgeAction: guardian.AgeAction(b.AccountAgeAction),
		VerifyMinAccountAgeHours: b.VerifyMinAccountAgeHours, ExemptRoleIDs: []snowflake.ID{},
	}
	var err error
	if s.AlertChannelID, err = snowflake.Parse(b.AlertChannelId); err != nil {
		return s, bad("alert channel")
	}
	if s.QuarantineRoleID, err = snowflake.Parse(b.QuarantineRoleId); err != nil {
		return s, bad("quarantine role")
	}
	if s.VerifyRoleID, err = snowflake.Parse(b.VerifyRoleId); err != nil {
		return s, bad("verified role")
	}
	for _, r := range b.ExemptRoleIds {
		id, err := snowflake.Parse(r)
		if err != nil {
			return s, bad("exempt role")
		}
		s.ExemptRoleIDs = append(s.ExemptRoleIDs, id)
	}
	return s, nil
}

func (h handlers) GetGuardianSettings(ctx context.Context, req apigen.GetGuardianSettingsRequestObject) (apigen.GetGuardianSettingsResponseObject, error) {
	gid, err := h.managed(ctx, req.GuildId)
	if err != nil {
		return nil, err
	}
	s, err := guardian.LoadSettings(ctx, h.config, gid)
	if err != nil {
		return nil, err
	}
	return apigen.GetGuardianSettings200JSONResponse(toAPIGuardianSettings(s)), nil
}

func (h handlers) UpdateGuardianSettings(ctx context.Context, req apigen.UpdateGuardianSettingsRequestObject) (apigen.UpdateGuardianSettingsResponseObject, error) {
	gid, err := h.managed(ctx, req.GuildId)
	if err != nil {
		return nil, err
	}
	if req.Body == nil {
		return nil, httpErr(http.StatusBadRequest, apigen.ErrorCodeBadRequest, "Missing request body.")
	}
	s, err := fromAPIGuardianSettings(*req.Body)
	if err != nil {
		return nil, err
	}
	if err := h.checkGuardianRefs(ctx, gid, s); err != nil {
		return nil, err
	}
	raw, _ := json.Marshal(s)
	actor := actorID(ctx)
	saved, err := h.config.SetSettings(ctx, gid, core.ModuleGuardian, raw, &actor, core.SourceDashboard)
	if errors.Is(err, core.ErrInvalidSettings) {
		msg := err.Error()
		if i := strings.LastIndex(msg, ": "); i >= 0 {
			msg = msg[i+2:]
		}
		return nil, httpErr(http.StatusBadRequest, apigen.ErrorCodeBadRequest, msg)
	}
	if err != nil {
		return nil, err
	}
	var out guardian.Settings
	if err := json.Unmarshal(saved, &out); err != nil {
		return nil, err
	}
	return apigen.UpdateGuardianSettings200JSONResponse(toAPIGuardianSettings(out)), nil
}

// checkGuardianRefs makes sure channels and roles belong to the server and
// that roles Niro must assign are not integration roles.
func (h handlers) checkGuardianRefs(ctx context.Context, gid snowflake.ID, s guardian.Settings) error {
	if s.AlertChannelID == 0 && s.QuarantineRoleID == 0 && s.VerifyRoleID == 0 && len(s.ExemptRoleIDs) == 0 {
		return nil
	}
	meta, err := h.guildMeta(ctx, gid)
	if err != nil {
		return err
	}
	bad := func(msg string) error { return httpErr(http.StatusBadRequest, apigen.ErrorCodeBadRequest, msg) }
	if s.AlertChannelID != 0 && !hasTextChannel(meta, s.AlertChannelID) {
		return bad("The alert channel must be a text or announcement channel in this server.")
	}
	role := func(id snowflake.ID) (dc.MetaRole, bool) {
		for _, r := range meta.Roles {
			if r.ID == id {
				return r, true
			}
		}
		return dc.MetaRole{}, false
	}
	for _, id := range []snowflake.ID{s.QuarantineRoleID, s.VerifyRoleID} {
		if id == 0 {
			continue
		}
		r, ok := role(id)
		if !ok || r.Managed || id == gid {
			return bad("Choose a normal role from this server (not @everyone or a bot role).")
		}
	}
	for _, id := range s.ExemptRoleIDs {
		if _, ok := role(id); !ok {
			return bad("An exempt role does not exist in this server.")
		}
	}
	return nil
}

// ---- events -----------------------------------------------------------------

func toAPIEvent(e guardian.Event) apigen.GuardianEvent {
	out := apigen.GuardianEvent{
		Id: e.ID, Kind: apigen.GuardianEventKind(e.Kind), Score: e.Score, Response: apigen.GuardianEventResponse(e.Response),
		Status: apigen.GuardianStatus(e.Status), CreatedAt: e.CreatedAt, ChannelIds: ids(e.Evidence.Channels),
		DeletedMessages: e.Evidence.Deleted, Domains: e.Evidence.Domains, Problems: e.Evidence.Problems,
		Monitor: e.Evidence.Monitor, ResolvedAt: e.ResolvedAt, Signals: []apigen.GuardianSignal{},
	}
	if out.Domains == nil {
		out.Domains = []string{}
	}
	if out.Problems == nil {
		out.Problems = []string{}
	}
	for _, s := range e.Signals {
		sig := apigen.GuardianSignal{Kind: apigen.GuardianSignalKind(s.Kind), Weight: s.Weight}
		if s.Detail != "" {
			d := s.Detail
			sig.Detail = &d
		}
		out.Signals = append(out.Signals, sig)
	}
	if e.UserID != 0 {
		id, name := e.UserID.String(), e.UserName
		out.UserId, out.UserName = &id, &name
	}
	if e.CaseNumber > 0 {
		n := e.CaseNumber
		out.CaseNumber = &n
	}
	if e.Evidence.Joins > 0 {
		n := e.Evidence.Joins
		out.Joins = &n
	}
	if e.Evidence.AccountAge != "" {
		a := e.Evidence.AccountAge
		out.AccountAge = &a
	}
	if e.ResolvedByName != "" {
		n := e.ResolvedByName
		out.ResolvedByName = &n
	}
	return out
}

func (h handlers) ListGuardianEvents(ctx context.Context, req apigen.ListGuardianEventsRequestObject) (apigen.ListGuardianEventsResponseObject, error) {
	gid, err := h.managed(ctx, req.GuildId)
	if err != nil {
		return nil, err
	}
	p := req.Params
	f := guardian.EventFilter{Limit: 25}
	if p.Limit != nil {
		f.Limit = *p.Limit
	}
	if p.Status != nil {
		f.Status = guardian.Status(*p.Status)
	}
	if p.Before != nil {
		f.Before = *p.Before
	}
	if f.UserID, err = optSnowflake(p.User); err != nil {
		return nil, httpErr(http.StatusBadRequest, apigen.ErrorCodeBadRequest, "Invalid user ID.")
	}
	events, err := h.guardian.ListEvents(ctx, gid, f)
	if err != nil {
		return nil, err
	}
	out := apigen.GuardianEventList{Items: make([]apigen.GuardianEvent, len(events))}
	for i, e := range events {
		out.Items[i] = toAPIEvent(e)
	}
	if len(events) == f.Limit {
		last := events[len(events)-1].ID
		out.NextBefore = &last
	}
	return apigen.ListGuardianEvents200JSONResponse(out), nil
}

func (h handlers) GetGuardianEvent(ctx context.Context, req apigen.GetGuardianEventRequestObject) (apigen.GetGuardianEventResponseObject, error) {
	gid, err := h.managed(ctx, req.GuildId)
	if err != nil {
		return nil, err
	}
	e, err := h.guardian.GetEvent(ctx, gid, req.EventId)
	if errors.Is(err, guardian.ErrEventNotFound) {
		return nil, httpErr(http.StatusNotFound, apigen.ErrorCodeNotFound, "Event not found.")
	}
	if err != nil {
		return nil, err
	}
	return apigen.GetGuardianEvent200JSONResponse(toAPIEvent(e)), nil
}

func (h handlers) ResolveGuardianEvent(ctx context.Context, req apigen.ResolveGuardianEventRequestObject) (apigen.ResolveGuardianEventResponseObject, error) {
	gid, err := h.managed(ctx, req.GuildId)
	if err != nil {
		return nil, err
	}
	if req.Body == nil {
		return nil, httpErr(http.StatusBadRequest, apigen.ErrorCodeBadRequest, "Missing request body.")
	}
	action := guardian.Resolution(req.Body.Action)
	if action != guardian.ResolveRestore && action != guardian.ResolveBan && action != guardian.ResolveKeep {
		return nil, httpErr(http.StatusBadRequest, apigen.ErrorCodeBadRequest, "Unknown action.")
	}
	params := dc.GuardianResolveParams{EventID: req.EventId, Action: action}
	if err := h.rpc.Call(ctx, dc.RPCGuardianResolve, gid, actorID(ctx), params, nil); err != nil {
		return nil, rpcErr(err)
	}
	e, err := h.guardian.GetEvent(ctx, gid, req.EventId)
	if err != nil {
		return nil, err
	}
	return apigen.ResolveGuardianEvent200JSONResponse(toAPIEvent(e)), nil
}

// ---- raid mode --------------------------------------------------------------

func (h handlers) raidStatus(ctx context.Context, gid snowflake.ID) (apigen.RaidStatus, error) {
	r, err := h.guardian.ActiveRaid(ctx, gid)
	if errors.Is(err, guardian.ErrNoRaid) {
		return apigen.RaidStatus{Active: false, Problems: []string{}}, nil
	}
	if err != nil {
		return apigen.RaidStatus{}, err
	}
	out := apigen.RaidStatus{Active: true, StartedAt: &r.StartedAt, EndsAt: &r.EndsAt, Manual: &r.Manual, Joins: &r.Joins, Problems: r.State.Problems}
	if out.Problems == nil {
		out.Problems = []string{}
	}
	return out, nil
}

func (h handlers) GetRaidStatus(ctx context.Context, req apigen.GetRaidStatusRequestObject) (apigen.GetRaidStatusResponseObject, error) {
	gid, err := h.managed(ctx, req.GuildId)
	if err != nil {
		return nil, err
	}
	st, err := h.raidStatus(ctx, gid)
	if err != nil {
		return nil, err
	}
	return apigen.GetRaidStatus200JSONResponse(st), nil
}

func (h handlers) StartRaidMode(ctx context.Context, req apigen.StartRaidModeRequestObject) (apigen.StartRaidModeResponseObject, error) {
	gid, err := h.managed(ctx, req.GuildId)
	if err != nil {
		return nil, err
	}
	if err := h.rpc.Call(ctx, dc.RPCGuardianRaidStart, gid, actorID(ctx), nil, nil); err != nil {
		return nil, rpcErr(err)
	}
	st, err := h.raidStatus(ctx, gid)
	if err != nil {
		return nil, err
	}
	return apigen.StartRaidMode200JSONResponse(st), nil
}

func (h handlers) EndRaidMode(ctx context.Context, req apigen.EndRaidModeRequestObject) (apigen.EndRaidModeResponseObject, error) {
	gid, err := h.managed(ctx, req.GuildId)
	if err != nil {
		return nil, err
	}
	if err := h.rpc.Call(ctx, dc.RPCGuardianRaidEnd, gid, actorID(ctx), nil, nil); err != nil {
		return nil, rpcErr(err)
	}
	st, err := h.raidStatus(ctx, gid)
	if err != nil {
		return nil, err
	}
	return apigen.EndRaidMode200JSONResponse(st), nil
}

// ---- audit ------------------------------------------------------------------

const auditTTL = time.Minute

func (h handlers) GetPermissionAudit(ctx context.Context, req apigen.GetPermissionAuditRequestObject) (apigen.GetPermissionAuditResponseObject, error) {
	gid, err := h.managed(ctx, req.GuildId)
	if err != nil {
		return nil, err
	}
	key := "niro:guardian-audit:" + gid.String()
	var res dc.AuditResult
	if found, err := h.kv.GetJSON(ctx, key, &res); err != nil || !found {
		if err := h.rpc.Call(ctx, dc.RPCGuardianAudit, gid, actorID(ctx), nil, &res); err != nil {
			return nil, rpcErr(err)
		}
		// The audit makes several Discord calls; a short cache keeps
		// repeated page loads cheap.
		if err := h.kv.SetJSON(ctx, key, res, auditTTL); err != nil {
			h.log.WarnContext(ctx, "caching audit failed", "err", err)
		}
	}
	out := apigen.AuditResult{WebhooksChecked: res.WebhooksChecked, Findings: make([]apigen.AuditFinding, len(res.Findings))}
	for i, f := range res.Findings {
		af := apigen.AuditFinding{Severity: apigen.AuditFindingSeverity(f.Severity), Code: apigen.AuditFindingCode(f.Code)}
		if f.SubjectID != 0 {
			id := f.SubjectID.String()
			af.SubjectId = &id
		}
		if f.Subject != "" {
			s := f.Subject
			af.Subject = &s
		}
		if len(f.Permissions) > 0 {
			p := f.Permissions
			af.Permissions = &p
		}
		if f.Count != 0 {
			n := f.Count
			af.Count = &n
		}
		if len(f.ChannelIDs) > 0 {
			c := ids(f.ChannelIDs)
			af.ChannelIds = &c
		}
		out.Findings[i] = af
	}
	return apigen.GetPermissionAudit200JSONResponse(out), nil
}
