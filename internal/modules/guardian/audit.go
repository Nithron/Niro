package guardian

import (
	"cmp"
	"slices"

	"github.com/disgoorg/snowflake/v2"
)

// Permission names used by the audit. The platform adapter translates its
// permission bits into these names.
const (
	PermAdministrator   = "administrator"
	PermManageGuild     = "manage_guild"
	PermManageRoles     = "manage_roles"
	PermManageChannels  = "manage_channels"
	PermManageWebhooks  = "manage_webhooks"
	PermBanMembers      = "ban_members"
	PermKickMembers     = "kick_members"
	PermModerateMembers = "moderate_members"
	PermManageMessages  = "manage_messages"
	PermMentionEveryone = "mention_everyone"
	PermManageExprs     = "manage_expressions"
)

// DangerousPermissions are the permissions the audit reports, most
// dangerous first.
var DangerousPermissions = []string{
	PermAdministrator, PermManageGuild, PermManageRoles, PermManageChannels, PermManageWebhooks,
	PermBanMembers, PermKickMembers, PermModerateMembers, PermManageMessages, PermMentionEveryone, PermManageExprs,
}

// Severity of an audit finding.
type Severity string

const (
	SeverityCritical Severity = "critical"
	SeverityHigh     Severity = "high"
	SeverityMedium   Severity = "medium"
	SeverityInfo     Severity = "info"
)

func severityRank(s Severity) int {
	return map[Severity]int{SeverityCritical: 0, SeverityHigh: 1, SeverityMedium: 2, SeverityInfo: 3}[s]
}

// AuditRole is a role as the audit sees it.
type AuditRole struct {
	ID          snowflake.ID
	Name        string
	Position    int
	Permissions []string // dangerous permissions only
	Managed     bool     // owned by an integration (bot role)
	Members     int      // -1 if unknown
}

// AuditBot is a bot member with its computed server-wide permissions.
type AuditBot struct {
	ID          snowflake.ID
	Name        string
	Permissions []string
	Self        bool // Niro itself
}

// AuditWebhook is an incoming webhook.
type AuditWebhook struct {
	ID        snowflake.ID
	Name      string
	ChannelID snowflake.ID
	CreatorID snowflake.ID
	Creator   string
}

// AuditInput is everything the audit looks at.
type AuditInput struct {
	EveryonePermissions []string
	Roles               []AuditRole
	Bots                []AuditBot
	Webhooks            []AuditWebhook
	VerificationLevel   int
	ModeratorMFA        bool // moderators must use 2FA
}

// Finding is one audit result. Code is a stable identifier (also the
// translation key suffix); the other fields fill in the message.
type Finding struct {
	Severity    Severity       `json:"severity"`
	Code        string         `json:"code"`
	SubjectID   snowflake.ID   `json:"subject_id,omitempty"`
	Subject     string         `json:"subject,omitempty"`
	Permissions []string       `json:"permissions,omitempty"`
	Count       int            `json:"count,omitempty"`
	ChannelIDs  []snowflake.ID `json:"channel_ids,omitempty"`
}

// Audit codes.
const (
	FindEveryoneDangerous = "everyone_dangerous"
	FindRoleAdmin         = "role_admin"
	FindRoleDangerous     = "role_dangerous"
	FindBotAdmin          = "bot_admin"
	FindBotDangerous      = "bot_dangerous"
	FindWebhooks          = "webhooks"
	FindVerificationNone  = "verification_none"
	FindVerificationLow   = "verification_low"
	FindNoModeratorMFA    = "no_moderator_mfa"
	FindSelfAdmin         = "self_admin"
)

// Audit flags risky configuration. Results are sorted by severity.
func Audit(in AuditInput) []Finding {
	var out []Finding
	critical := []string{PermAdministrator, PermManageGuild, PermManageRoles, PermManageChannels, PermManageWebhooks, PermBanMembers, PermKickMembers}

	if p := ordered(in.EveryonePermissions); len(p) > 0 {
		sev := SeverityHigh
		if anyOf(p, critical) {
			sev = SeverityCritical
		}
		out = append(out, Finding{Severity: sev, Code: FindEveryoneDangerous, Permissions: p})
	}

	for _, r := range in.Roles {
		p := ordered(r.Permissions)
		if len(p) == 0 || r.Managed {
			continue // bot roles are reported per bot below
		}
		switch {
		case slices.Contains(p, PermAdministrator):
			out = append(out, Finding{Severity: SeverityHigh, Code: FindRoleAdmin, SubjectID: r.ID, Subject: r.Name, Count: r.Members})
		case anyOf(p, critical):
			out = append(out, Finding{Severity: SeverityMedium, Code: FindRoleDangerous, SubjectID: r.ID, Subject: r.Name, Permissions: p, Count: r.Members})
		}
	}

	for _, b := range in.Bots {
		p := ordered(b.Permissions)
		switch {
		case b.Self && slices.Contains(p, PermAdministrator):
			out = append(out, Finding{Severity: SeverityMedium, Code: FindSelfAdmin, SubjectID: b.ID, Subject: b.Name})
		case b.Self:
		case slices.Contains(p, PermAdministrator):
			out = append(out, Finding{Severity: SeverityHigh, Code: FindBotAdmin, SubjectID: b.ID, Subject: b.Name})
		case anyOf(p, critical):
			out = append(out, Finding{Severity: SeverityMedium, Code: FindBotDangerous, SubjectID: b.ID, Subject: b.Name, Permissions: p})
		}
	}

	if len(in.Webhooks) > 0 {
		var chans []snowflake.ID
		for _, w := range in.Webhooks {
			if !slices.Contains(chans, w.ChannelID) {
				chans = append(chans, w.ChannelID)
			}
		}
		out = append(out, Finding{Severity: SeverityInfo, Code: FindWebhooks, Count: len(in.Webhooks), ChannelIDs: chans})
	}

	switch {
	case in.VerificationLevel == 0:
		out = append(out, Finding{Severity: SeverityMedium, Code: FindVerificationNone})
	case in.VerificationLevel == 1:
		out = append(out, Finding{Severity: SeverityInfo, Code: FindVerificationLow})
	}
	if !in.ModeratorMFA {
		out = append(out, Finding{Severity: SeverityMedium, Code: FindNoModeratorMFA})
	}

	slices.SortStableFunc(out, func(a, b Finding) int {
		return cmp.Compare(severityRank(a.Severity), severityRank(b.Severity))
	})
	return out
}

// ordered keeps only dangerous permissions, in DangerousPermissions order.
func ordered(perms []string) []string {
	var out []string
	for _, p := range DangerousPermissions {
		if slices.Contains(perms, p) {
			out = append(out, p)
		}
	}
	return out
}

func anyOf(have, want []string) bool {
	for _, w := range want {
		if slices.Contains(have, w) {
			return true
		}
	}
	return false
}
