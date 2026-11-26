package main

import (
	"encoding/json"
	"net/http"
	"regexp"
	"strings"
	"sync/atomic"
	"time"

	"github.com/disgoorg/disgo/discord"
	"github.com/disgoorg/snowflake/v2"
)

// fakeDiscord answers the REST calls the bot makes with plausible, minimal
// responses, as fast as it can. It keeps no state beyond counters.
type fakeDiscord struct {
	appID    snowflake.ID
	requests atomic.Int64
	ids      atomic.Uint64
}

func newFakeDiscord() *fakeDiscord {
	return &fakeDiscord{appID: snowflake.New(time.Now().AddDate(-1, 0, 0))}
}

var (
	reMessages  = regexp.MustCompile(`^/channels/(\d+)/messages$`)
	reMessage   = regexp.MustCompile(`^/channels/(\d+)/messages/(\d+)$`)
	reMember    = regexp.MustCompile(`^/guilds/(\d+)/members/(\d+)$`)
	reUser      = regexp.MustCompile(`^/users/(\d+)$`)
	reAuditLog  = regexp.MustCompile(`^/guilds/\d+/audit-logs$`)
	reDMChannel = regexp.MustCompile(`^/users/@me/channels$`)
	reList      = regexp.MustCompile(`/(invites|roles|channels|bans|webhooks|threads|rules|emojis)$`)
)

func (f *fakeDiscord) newID() snowflake.ID {
	return snowflake.ID(uint64(snowflake.New(time.Now())) | (f.ids.Add(1) & 0x3FFFFF))
}

func (f *fakeDiscord) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	f.requests.Add(1)
	path := strings.TrimPrefix(r.URL.Path, "/api/v10")
	w.Header().Set("Content-Type", "application/json")
	now := time.Now().UTC().Format(time.RFC3339Nano)
	bot := map[string]any{"id": f.appID, "username": "Niro", "discriminator": "0", "bot": true}
	write := func(v any) { _ = json.NewEncoder(w).Encode(v) }

	switch {
	case r.Method == http.MethodGet && path == "/applications/@me":
		write(map[string]any{"id": f.appID, "name": "Niro Soak", "description": "", "bot_public": false, "bot_require_code_grant": false,
			"verify_key": "", "flags": int(discord.ApplicationFlagGatewayGuildMembers | discord.ApplicationFlagGatewayMessageContent), "bot": bot})
	case r.Method == http.MethodPost && reMessages.MatchString(path):
		ch := reMessages.FindStringSubmatch(path)[1]
		write(map[string]any{"id": f.newID(), "channel_id": ch, "content": "", "timestamp": now, "type": 0, "author": bot,
			"mentions": []any{}, "attachments": []any{}, "embeds": []any{}, "mention_roles": []any{}})
	case (r.Method == http.MethodGet || r.Method == http.MethodPatch) && reMessage.MatchString(path):
		m := reMessage.FindStringSubmatch(path)
		write(map[string]any{"id": m[2], "channel_id": m[1], "content": "", "timestamp": now, "type": 0, "author": bot,
			"mentions": []any{}, "attachments": []any{}, "embeds": []any{}, "mention_roles": []any{}})
	case (r.Method == http.MethodGet || r.Method == http.MethodPatch) && reMember.MatchString(path):
		m := reMember.FindStringSubmatch(path)
		write(map[string]any{"user": map[string]any{"id": m[2], "username": "member"}, "roles": []any{}, "joined_at": "2023-01-01T00:00:00Z"})
	case r.Method == http.MethodGet && reUser.MatchString(path):
		write(map[string]any{"id": reUser.FindStringSubmatch(path)[1], "username": "user"})
	case r.Method == http.MethodGet && reAuditLog.MatchString(path):
		write(map[string]any{"audit_log_entries": []any{}, "users": []any{}, "integrations": []any{}, "webhooks": []any{},
			"threads": []any{}, "application_commands": []any{}, "auto_moderation_rules": []any{}, "guild_scheduled_events": []any{}})
	case r.Method == http.MethodPost && reDMChannel.MatchString(path):
		write(map[string]any{"id": f.newID(), "type": 1, "recipients": []any{bot}})
	case r.Method == http.MethodGet && reList.MatchString(path):
		write([]any{})
	case r.Method == http.MethodGet || r.Method == http.MethodPost:
		write(map[string]any{})
	default: // PUT, PATCH, DELETE
		w.WriteHeader(http.StatusNoContent)
	}
}
