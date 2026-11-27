package store

import (
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"testing"
)

// guildTables hold one server's data. A query that reads or changes them by
// anything but time must filter on guild_id, so an ID from one server can
// never reach another server's rows.
var guildTables = []string{
	"guild_configs", "config_audit", "guild_case_counters", "mod_cases", "scheduled_actions",
	"guardian_events", "guardian_quarantines", "guardian_raids", "channel_lockdowns",
	"role_panels", "ticket_panels", "guild_ticket_counters", "tickets", "ticket_transcripts",
}

// unscoped are the queries allowed without a guild_id filter, and why.
var unscoped = map[string]string{
	"ClaimDueActions":           "the scheduler claims due work across servers; each row carries its guild_id",
	"CompleteScheduledAction":   "by the primary key of a row ClaimDueActions returned",
	"FailScheduledAction":       "by the primary key of a row ClaimDueActions returned",
	"PurgeGuardianEvents":       "retention by age",
	"PurgeTranscripts":          "retention by age",
	"PurgeDoneScheduledActions": "retention by age",
	"PurgeEndedRaids":           "retention by age",
	"PurgeConfigAudit":          "retention by age",
	"DueRaids":                  "the raid timer ends raids across servers; each row carries its guild_id",
}

var nameRe = regexp.MustCompile(`(?m)^-- name: (\w+)`)

func TestQueriesAreScopedToAGuild(t *testing.T) {
	files, _ := filepath.Glob("queries/*.sql")
	var bad []string
	seen := map[string]bool{}
	for _, f := range files {
		b, err := os.ReadFile(f)
		if err != nil {
			t.Fatal(err)
		}
		src := string(b)
		locs := nameRe.FindAllStringSubmatchIndex(src, -1)
		for i, loc := range locs {
			end := len(src)
			if i+1 < len(locs) {
				end = locs[i+1][0]
			}
			name, body := src[loc[2]:loc[3]], strings.ToLower(src[loc[1]:end])
			seen[name] = true
			if !touchesGuildTable(body) || strings.Contains(body, "guild_id") {
				continue
			}
			if !strings.Contains(body, "where") && !strings.Contains(body, "update") && !strings.Contains(body, "delete") {
				continue // inserts carry the guild ID as a value
			}
			if _, ok := unscoped[name]; !ok {
				bad = append(bad, filepath.Base(f)+": "+name)
			}
		}
	}
	sort.Strings(bad)
	if len(bad) > 0 {
		t.Errorf("queries on server tables without a guild_id filter:\n  %s", strings.Join(bad, "\n  "))
	}
	for name := range unscoped {
		if !seen[name] {
			t.Errorf("allow-list entry %s matches no query; remove it", name)
		}
	}
}

func touchesGuildTable(body string) bool {
	for _, tbl := range guildTables {
		if regexp.MustCompile(`\b` + tbl + `\b`).MatchString(body) {
			return true
		}
	}
	return false
}
