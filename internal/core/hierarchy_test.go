package core

import (
	"errors"
	"testing"
)

func TestCanActOn(t *testing.T) {
	owner := Rank{ID: 1, IsOwner: true, TopRolePosition: 0}
	admin := Rank{ID: 2, TopRolePosition: 10}
	mod := Rank{ID: 3, TopRolePosition: 5}
	member := Rank{ID: 4, TopRolePosition: 1}
	peerMod := Rank{ID: 5, TopRolePosition: 5}
	bot := Rank{ID: 9, TopRolePosition: 8}
	lowBot := Rank{ID: 9, TopRolePosition: 1}

	tests := []struct {
		name          string
		actor, target Rank
		bot           Rank
		want          error
	}{
		{"mod on member", mod, member, bot, nil},
		{"owner on admin above the bot is blocked by the bot rank", owner, admin, bot, ErrBotRankTooLow},
		{"owner on member", owner, member, bot, nil},
		{"admin on mod", admin, mod, bot, nil},
		{"self", mod, mod, bot, ErrTargetIsSelf},
		{"target is owner", admin, owner, bot, ErrTargetIsOwner},
		{"equal rank", mod, peerMod, bot, ErrActorRankTooLow},
		{"lower actor", member, mod, bot, ErrActorRankTooLow},
		{"bot below target", mod, member, Rank{ID: 9, TopRolePosition: 1}, ErrBotRankTooLow},
		{"bot equal to target", admin, Rank{ID: 6, TopRolePosition: 1}, lowBot, ErrBotRankTooLow},
		{"target is the bot", admin, bot, bot, ErrTargetIsBot},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := CanActOn(tt.actor, tt.target, tt.bot)
			if !errors.Is(got, tt.want) {
				t.Fatalf("CanActOn() = %v, want %v", got, tt.want)
			}
		})
	}
}
