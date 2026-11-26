package discord

import (
	"net/url"
	"strconv"

	"github.com/disgoorg/disgo/discord"
	"github.com/disgoorg/snowflake/v2"
)

// InvitePermissions is the v0.1 permission set requested when a server adds
// Niro (docs/plan.md §6). Deliberately no Administrator.
const InvitePermissions = discord.PermissionViewChannel |
	discord.PermissionSendMessages |
	discord.PermissionEmbedLinks |
	discord.PermissionAttachFiles |
	discord.PermissionReadMessageHistory |
	discord.PermissionManageMessages |
	discord.PermissionModerateMembers |
	discord.PermissionKickMembers |
	discord.PermissionBanMembers |
	discord.PermissionManageRoles |
	discord.PermissionManageChannels |
	discord.PermissionManageThreads |
	discord.PermissionCreatePrivateThreads |
	discord.PermissionSendMessagesInThreads | // thread-mode tickets (decision 25)
	discord.PermissionViewAuditLog |
	discord.PermissionManageGuild

// InviteURL is the install link for the bot. If guildID is set, Discord
// preselects that server.
func InviteURL(clientID, guildID snowflake.ID) string {
	q := url.Values{}
	q.Set("client_id", clientID.String())
	q.Set("scope", "bot applications.commands")
	q.Set("permissions", strconv.FormatInt(int64(InvitePermissions), 10))
	q.Set("integration_type", "0") // guild install
	if guildID != 0 {
		q.Set("guild_id", guildID.String())
		q.Set("disable_guild_select", "true")
	}
	return "https://discord.com/oauth2/authorize?" + q.Encode()
}

// CanManageGuild mirrors who may configure Niro for a server: the owner,
// Administrators, and members with Manage Server.
func CanManageGuild(owner bool, perms discord.Permissions) bool {
	return owner || perms.Has(discord.PermissionAdministrator) || perms.Has(discord.PermissionManageGuild)
}
