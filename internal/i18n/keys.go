package i18n

// Module names and descriptions use dynamic keys: module.<id>.name/.description.
// The catalog test checks them for every default module.

func ModuleName(id string) Key        { return Key("module." + id + ".name") }
func ModuleDescription(id string) Key { return Key("module." + id + ".description") }

var (
	CmdPingDescription = define("cmd.ping.description")
	CmdPingReply       = define("cmd.ping.reply")

	CmdAboutDescription = define("cmd.about.description")
	CmdAboutTitle       = define("cmd.about.title")
	CmdAboutBody        = define("cmd.about.body")
	CmdAboutDashboard   = define("cmd.about.dashboard")
	CmdAboutWebsite     = define("cmd.about.website")

	CmdModulesDescription = define("cmd.modules.description")
	CmdModulesTitle       = define("cmd.modules.title")
	CmdModulesEnabled     = define("cmd.modules.enabled")
	CmdModulesDisabled    = define("cmd.modules.disabled")
	CmdModulesAlwaysOn    = define("cmd.modules.always_on")
	CmdModulesManage      = define("cmd.modules.manage")
	CmdModulesFooter      = define("cmd.modules.footer")

	ErrorGeneric   = define("error.generic")
	ErrorGuildOnly = define("error.guild_only")
)
