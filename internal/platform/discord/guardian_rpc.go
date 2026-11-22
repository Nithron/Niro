package discord

import (
	"context"
	"errors"

	"github.com/disgoorg/disgo/rest"
	"github.com/disgoorg/snowflake/v2"

	"github.com/nithron/niro/internal/core"
	"github.com/nithron/niro/internal/modules/guardian"
	"github.com/nithron/niro/internal/modules/moderation"
	"github.com/nithron/niro/internal/rpc"
)

// Guardian RPC methods (shared with the API).
const (
	RPCGuardianResolve   = "guardian.resolve"
	RPCGuardianRaidStart = "guardian.raid.start"
	RPCGuardianRaidEnd   = "guardian.raid.end"
	RPCGuardianAudit     = "guardian.audit"
)

type GuardianResolveParams struct {
	EventID int64               `json:"event_id"`
	Action  guardian.Resolution `json:"action"`
}

// GuardianRaidResult describes the raid after starting it.
type GuardianRaidResult struct {
	StartedAt string   `json:"started_at"`
	EndsAt    string   `json:"ends_at"`
	Problems  []string `json:"problems"`
}

func (b *Bot) registerGuardianRPC(srv *rpc.Server) {
	srv.Handle(RPCGuardianResolve, func(ctx context.Context, r rpc.Request) (any, error) {
		var p GuardianResolveParams
		if err := r.Bind(&p); err != nil {
			return nil, err
		}
		_, err := b.guard.Resolve(ctx, r.GuildID, p.EventID, p.Action, b.rpcActor(ctx, r.GuildID, r.ActorID))
		return nil, guardianRPCError(err)
	})
	srv.Handle(RPCGuardianRaidStart, func(ctx context.Context, r rpc.Request) (any, error) {
		raid, err := b.guard.StartRaid(ctx, r.GuildID, b.rpcActor(ctx, r.GuildID, r.ActorID))
		if err != nil {
			return nil, guardianRPCError(err)
		}
		problems := raid.State.Problems
		if problems == nil {
			problems = []string{}
		}
		return GuardianRaidResult{StartedAt: raid.StartedAt.UTC().Format(timeLayout), EndsAt: raid.EndsAt.UTC().Format(timeLayout), Problems: problems}, nil
	})
	srv.Handle(RPCGuardianRaidEnd, func(ctx context.Context, r rpc.Request) (any, error) {
		_, err := b.guard.EndRaid(ctx, r.GuildID, b.rpcActor(ctx, r.GuildID, r.ActorID))
		return nil, guardianRPCError(err)
	})
	srv.Handle(RPCGuardianAudit, func(ctx context.Context, r rpc.Request) (any, error) {
		res, err := b.runAudit(ctx, r.GuildID)
		if err != nil {
			return nil, discordRPCError(err)
		}
		if res.Findings == nil {
			res.Findings = []guardian.Finding{}
		}
		return res, nil
	})
}

const timeLayout = "2006-01-02T15:04:05Z07:00"

// rpcActor describes a dashboard user as a moderation actor, with their rank
// in the server so hierarchy checks apply to dashboard actions too.
func (b *Bot) rpcActor(ctx context.Context, guildID, userID snowflake.ID) moderation.Actor {
	a := moderation.Actor{Rank: core.Rank{ID: userID}, Name: userID.String()}
	m, ok := b.client.Caches.Member(guildID, userID)
	if !ok {
		rm, err := b.client.Rest.GetMember(guildID, userID, rest.WithCtx(ctx))
		if err != nil {
			return a
		}
		m = *rm
	}
	return moderation.Actor{Rank: rankOf(b.client, guildID, userID, m.RoleIDs), Name: displayName(m.User, m.Nick)}
}

func guardianRPCError(err error) error {
	var ae *moderation.ActionError
	switch {
	case err == nil:
		return nil
	case errors.Is(err, guardian.ErrEventNotFound):
		return rpc.Errorf(rpc.CodeNotFound, "event not found")
	case errors.Is(err, guardian.ErrAlreadyResolved):
		return rpc.Errorf(rpc.CodeInvalid, "another moderator already handled this event")
	case errors.Is(err, guardian.ErrBadResolution):
		return rpc.Errorf(rpc.CodeInvalid, "that action is not available for this event")
	case errors.Is(err, guardian.ErrRaidActive):
		return rpc.Errorf(rpc.CodeInvalid, "raid mode is already on")
	case errors.Is(err, guardian.ErrNoRaid):
		return rpc.Errorf(rpc.CodeInvalid, "raid mode is not on")
	case errors.Is(err, guardian.ErrModuleOff):
		return rpc.Errorf(rpc.CodeInvalid, "the Guardian module is turned off")
	case errors.Is(err, core.ErrActorRankTooLow), errors.Is(err, core.ErrBotRankTooLow), errors.Is(err, core.ErrTargetIsOwner):
		return rpc.Errorf(rpc.CodeInvalid, "%s", err.Error())
	case errors.As(err, &ae):
		return discordRPCError(ae.Err)
	}
	return err
}
