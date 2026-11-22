package moderation

import (
	"context"
	"fmt"
	"regexp"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/disgoorg/snowflake/v2"
)

// PurgeFilter narrows which messages a purge deletes. All set conditions must match.
type PurgeFilter struct {
	UserID    snowflake.ID // only this author
	BotsOnly  bool         // only messages from bots
	LinksOnly bool         // only messages containing links
}

// PurgeRequest deletes recent messages in one channel.
type PurgeRequest struct {
	GuildID   snowflake.ID
	ChannelID snowflake.ID
	Actor     Actor
	Count     int
	Filter    PurgeFilter
	Reason    string
}

// Discord's bulk delete only accepts messages younger than 14 days; we stay
// a little under to avoid races at the boundary.
const bulkDeleteMaxAge = 14*24*time.Hour - time.Hour

// scanLimit bounds how far back a filtered purge looks for matches.
const scanLimit = 500

var linkRE = regexp.MustCompile(`(?i)\b(?:https?://|www\.)\S+|\bdiscord(?:app)?\.(?:gg|com/invite)/\S+`)

// HasLink reports whether text contains a URL or invite.
func HasLink(text string) bool { return linkRE.MatchString(text) }

func (f PurgeFilter) matches(m Message) bool {
	if m.Pinned {
		return false // never purge pinned messages
	}
	if f.UserID != 0 && m.AuthorID != f.UserID {
		return false
	}
	if f.BotsOnly && !m.AuthorBot {
		return false
	}
	if f.LinksOnly && !HasLink(m.Content) && !m.HasEmbeds {
		return false
	}
	return true
}

// Purge deletes up to Count matching recent messages and records a case.
// Messages older than 14 days are skipped (Discord can't bulk-delete them).
func (s *Service) Purge(ctx context.Context, req PurgeRequest) (Outcome, error) {
	if req.Count < 1 || req.Count > MaxPurge {
		return Outcome{}, ErrBadPurge
	}
	reason := strings.TrimSpace(req.Reason)
	if utf8.RuneCountInString(reason) > MaxReasonLength {
		return Outcome{}, ErrReasonTooLong
	}
	settings, err := s.settings(ctx, req.GuildID)
	if err != nil {
		return Outcome{}, err
	}
	if reason == "" && settings.RequireReason {
		return Outcome{}, ErrReasonRequired
	}

	cutoff := s.now().Add(-bulkDeleteMaxAge)
	var ids []snowflake.ID
	var before snowflake.ID
	scanned := 0
scan:
	for scanned < scanLimit && len(ids) < req.Count {
		page, err := s.platform.RecentMessages(ctx, req.ChannelID, before, 100)
		if err != nil {
			return Outcome{}, fmt.Errorf("reading messages: %w", err)
		}
		if len(page) == 0 {
			break
		}
		for _, m := range page {
			scanned++
			if m.CreatedAt.Before(cutoff) {
				break scan // older messages can't be bulk deleted
			}
			if req.Filter.matches(m) {
				ids = append(ids, m.ID)
				if len(ids) == req.Count {
					break scan
				}
			}
		}
		before = page[len(page)-1].ID
		if len(page) < 100 {
			break
		}
	}

	if len(ids) > 0 {
		if err := s.platform.DeleteMessages(ctx, req.ChannelID, ids, auditReason(Request{Actor: req.Actor, Reason: reason})); err != nil {
			return Outcome{}, &ActionError{Action: ActionPurge, Err: err}
		}
	}

	nc := NewCase{
		GuildID: req.GuildID, Action: ActionPurge, TargetID: req.Filter.UserID,
		ModeratorID: req.Actor.ID, ModeratorName: req.Actor.Name, Reason: reason, Source: SourceManual,
		Details: Details{ChannelID: req.ChannelID, Deleted: len(ids)},
	}
	if req.Filter.UserID != 0 {
		if m, err := s.platform.Member(ctx, req.GuildID, req.Filter.UserID); err == nil {
			nc.TargetName = m.Name
		}
	}
	c, err := s.store.CreateCase(ctx, nc)
	if err != nil {
		return Outcome{}, fmt.Errorf("messages deleted but recording the case failed: %w", err)
	}
	out := Outcome{Case: c}
	out.LogPosted = s.postLog(ctx, settings, &out.Case)
	return out, nil
}
