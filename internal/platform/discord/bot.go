// Package discord is Nithro's Discord adapter, built on disgo. It translates
// gateway events and interactions into calls on the platform-agnostic core,
// and core results into Discord messages (Components V2).
package discord

import (
	"context"
	"fmt"
	"log/slog"
	"net/url"
	"time"

	"github.com/disgoorg/disgo"
	"github.com/disgoorg/disgo/bot"
	"github.com/disgoorg/disgo/cache"
	"github.com/disgoorg/disgo/discord"
	"github.com/disgoorg/disgo/events"
	"github.com/disgoorg/disgo/gateway"
	"github.com/disgoorg/disgo/handler"
	"github.com/disgoorg/disgo/sharding"
	"github.com/disgoorg/snowflake/v2"
	"github.com/prometheus/client_golang/prometheus"

	"github.com/nithron/nithro/internal/core"
	"github.com/nithron/nithro/internal/i18n"
	"github.com/nithron/nithro/internal/modules/moderation"
)

// Intents are the gateway intents the bot requests. Each milestone adds only
// what its features need (least privilege); see docs/plan.md §6.
const Intents = gateway.IntentGuilds

// Deps are the bot's collaborators.
type Deps struct {
	Token        string
	DevGuildID   snowflake.ID // register commands here instead of globally
	DashboardURL *url.URL
	Version      string

	Log     *slog.Logger
	Config  *core.ConfigService
	Guilds  core.GuildDirectory
	Cases   moderation.CaseStore
	Catalog *i18n.Catalog
	Metrics prometheus.Registerer
}

// Bot is a running Nithro Discord client.
type Bot struct {
	deps    Deps
	log     *slog.Logger
	client  *bot.Client
	emojis  *Emojis
	metrics *metrics
	mod     *moderation.Service
	pending pendingBans
}

// New builds the client. Nothing connects until Run.
func New(deps Deps) (*Bot, error) {
	b := &Bot{deps: deps, log: deps.Log.With("component", "bot")}
	var err error
	if b.metrics, err = newMetrics(deps.Metrics); err != nil {
		return nil, err
	}

	router := handler.New()
	router.Use(b.observe)
	router.SlashCommand("/ping", b.handlePing)
	router.SlashCommand("/about", b.handleAbout)
	router.SlashCommand("/modules", b.handleModules)
	b.registerModeration(router)
	router.Error(b.handleError)
	router.NotFound(func(e *handler.InteractionEvent) error {
		b.log.Warn("unhandled interaction", "type", e.Type())
		return nil
	})

	b.client, err = disgo.New(deps.Token,
		bot.WithLogger(deps.Log.With("component", "disgo")),
		bot.WithShardManagerConfigOpts(
			sharding.WithAutoScaling(true), // reshard automatically when Discord asks
			sharding.WithGatewayConfigOpts(gateway.WithIntents(Intents)),
		),
		bot.WithCacheConfigOpts(cache.WithCaches(cache.FlagGuilds, cache.FlagRoles, cache.FlagChannels, cache.FlagMembers)),
		bot.WithEventListeners(router, &events.ListenerAdapter{
			OnGuildReady: func(e *events.GuildReady) { b.recordGuild(e.Guild) },
			OnGuildJoin:  func(e *events.GuildJoin) { b.recordGuild(e.Guild); b.log.Info("joined guild", "guild_id", e.GuildID) },
			OnGuildLeave: func(e *events.GuildLeave) { b.forgetGuild(e.GuildID) },
		}),
	)
	if err != nil {
		return nil, fmt.Errorf("building discord client: %w", err)
	}
	b.emojis = NewEmojis(b.client.Rest, b.client.ApplicationID, b.log)
	b.mod = moderation.NewService(deps.Cases, modPlatform{client: b.client},
		modNotifier{client: b.client, p: b.presenter(), locale: b.guildLocale},
		func(ctx context.Context, guildID snowflake.ID) (moderation.Settings, error) {
			return moderation.LoadSettings(ctx, deps.Config, guildID)
		}, b.log.With("module", "moderation"))
	return b, nil
}

// Moderation exposes the moderation service (used by the RPC handlers).
func (b *Bot) Moderation() *moderation.Service { return b.mod }

func (b *Bot) presenter() presenter { return presenter{cat: b.deps.Catalog, emojis: b.emojis} }

// guildLocale is the language for public messages in a guild (logs, DMs).
func (b *Bot) guildLocale(guildID snowflake.ID) string {
	if g, ok := b.client.Caches.Guild(guildID); ok && g.PreferredLocale != "" {
		return g.PreferredLocale
	}
	return "en"
}

// runScheduler lifts temporary bans once the bot knows its own identity.
func (b *Bot) runScheduler(ctx context.Context) {
	for {
		if self, ok := b.client.Caches.SelfUser(); ok {
			actor := moderation.Actor{Rank: core.Rank{ID: self.ID}, Name: self.Username}
			b.mod.RunScheduler(ctx, actor, 30*time.Second)
			return
		}
		select {
		case <-ctx.Done():
			return
		case <-time.After(time.Second):
		}
	}
}

// Run syncs commands and emojis, connects all shards, and blocks until ctx is done.
func (b *Bot) Run(ctx context.Context) error {
	if err := b.syncCommands(); err != nil {
		return err
	}
	if err := b.emojis.Sync(ctx); err != nil {
		// Not fatal: messages fall back to Unicode emoji.
		b.log.Warn("syncing application emojis failed", "err", err)
	}

	if err := b.client.OpenShardManager(ctx); err != nil {
		return fmt.Errorf("connecting to the gateway: %w", err)
	}
	b.log.Info("bot online", "application_id", b.client.ApplicationID, "version", b.deps.Version)
	go b.runScheduler(ctx)

	<-ctx.Done()
	closeCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	b.client.Close(closeCtx)
	b.log.Info("bot stopped")
	return nil
}

// Ready reports whether at least one shard is connected (readiness check).
func (b *Bot) Ready(context.Context) error {
	for s := range b.client.ShardManager.Shards() {
		if s.Status() == gateway.StatusReady {
			return nil
		}
	}
	return fmt.Errorf("no shard is connected")
}

func (b *Bot) syncCommands() error {
	cmds := b.commands()
	if b.deps.DevGuildID != 0 {
		if _, err := b.client.Rest.SetGuildCommands(b.client.ApplicationID, b.deps.DevGuildID, cmds); err != nil {
			return fmt.Errorf("registering commands in dev guild %s: %w", b.deps.DevGuildID, err)
		}
		b.log.Info("commands registered in dev guild", "guild_id", b.deps.DevGuildID, "count", len(cmds))
		return nil
	}
	if _, err := b.client.Rest.SetGlobalCommands(b.client.ApplicationID, cmds); err != nil {
		return fmt.Errorf("registering global commands: %w", err)
	}
	b.log.Info("global commands registered", "count", len(cmds))
	return nil
}

func (b *Bot) recordGuild(g discord.CacheGuild) {
	info := core.GuildInfo{ID: g.ID, Name: g.Name, OwnerID: g.OwnerID, Locale: g.PreferredLocale}
	if g.Icon != nil {
		info.Icon = *g.Icon
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := b.deps.Guilds.UpsertGuild(ctx, info); err != nil {
		b.log.Error("recording guild failed", "guild_id", g.ID, "err", err)
		return
	}
	b.metrics.guilds.Set(float64(b.client.Caches.GuildsLen()))
}

func (b *Bot) forgetGuild(id snowflake.ID) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := b.deps.Guilds.MarkGuildLeft(ctx, id); err != nil {
		b.log.Error("marking guild as left failed", "guild_id", id, "err", err)
	}
	b.metrics.guilds.Set(float64(b.client.Caches.GuildsLen()))
	b.log.Info("left guild", "guild_id", id)
}
