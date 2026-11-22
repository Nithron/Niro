package discord

import (
	"github.com/disgoorg/disgo/discord"
	"github.com/disgoorg/omit"
	"golang.org/x/text/language"

	"github.com/nithron/nithro/internal/i18n"
)

// commands returns the slash commands to register. Descriptions come from the
// i18n catalogs (English is the default; other catalog languages become
// Discord localizations automatically).
func (b *Bot) commands() []discord.ApplicationCommandCreate {
	guildOnly := []discord.InteractionContextType{discord.InteractionContextTypeGuild}
	guildInstall := []discord.ApplicationIntegrationType{discord.ApplicationIntegrationTypeGuildInstall}
	manageGuild := discord.PermissionManageGuild

	desc := func(k i18n.Key) (string, map[discord.Locale]string) {
		return b.deps.Catalog.T("en", k, nil), localizations(b.deps.Catalog, k)
	}
	slash := func(name string, k i18n.Key) discord.SlashCommandCreate {
		d, loc := desc(k)
		return discord.SlashCommandCreate{
			Name: name, Description: d, DescriptionLocalizations: loc,
			Contexts: guildOnly, IntegrationTypes: guildInstall,
		}
	}

	modules := slash("modules", i18n.CmdModulesDescription)
	modules.DefaultMemberPermissions = omit.New(&manageGuild)

	return []discord.ApplicationCommandCreate{
		slash("ping", i18n.CmdPingDescription),
		slash("about", i18n.CmdAboutDescription),
		modules,
	}
}

// localizations maps a key's translations onto Discord's locale codes,
// matching exactly ("nl") or by base language ("pt" serves "pt-BR").
func localizations(c *i18n.Catalog, k i18n.Key) map[discord.Locale]string {
	tr := c.Translations(k)
	if len(tr) == 0 {
		return nil
	}
	out := map[discord.Locale]string{}
	for loc := range discord.Locales {
		tag, err := language.Parse(loc.Code())
		if err != nil {
			continue
		}
		if s, ok := tr[tag]; ok {
			out[loc] = s
			continue
		}
		base, _ := tag.Base()
		for t, s := range tr {
			if tb, _ := t.Base(); tb == base {
				out[loc] = s
				break
			}
		}
	}
	return out
}
