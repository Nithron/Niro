package core

import (
	"errors"

	"github.com/disgoorg/snowflake/v2"
)

// Rank is what the hierarchy rule needs to know about a member: who they are,
// whether they own the server, and the position of their highest role.
// It is platform-agnostic; adapters fill it from their own member types.
type Rank struct {
	ID              snowflake.ID
	IsOwner         bool
	TopRolePosition int
}

var (
	ErrTargetIsSelf    = errors.New("cannot act on yourself")
	ErrTargetIsOwner   = errors.New("cannot act on the server owner")
	ErrTargetIsBot     = errors.New("cannot act on the bot itself")
	ErrActorRankTooLow = errors.New("actor's highest role must be above the target's highest role")
	ErrBotRankTooLow   = errors.New("bot's highest role must be above the target's highest role")
)

// CanActOn reports whether actor may moderate target, with the bot carrying out
// the action. It mirrors Discord's own rules so we fail with a clear reason
// before Discord rejects the request:
//   - nobody can act on themselves, on the owner, or on the bot itself;
//   - the owner outranks everyone; anyone else needs a strictly higher top role;
//   - the bot's own top role must also be strictly above the target's.
func CanActOn(actor, target, bot Rank) error {
	switch {
	case actor.ID == target.ID:
		return ErrTargetIsSelf
	case target.IsOwner:
		return ErrTargetIsOwner
	case target.ID == bot.ID:
		return ErrTargetIsBot
	case !actor.IsOwner && actor.TopRolePosition <= target.TopRolePosition:
		return ErrActorRankTooLow
	case bot.TopRolePosition <= target.TopRolePosition:
		return ErrBotRankTooLow
	}
	return nil
}
