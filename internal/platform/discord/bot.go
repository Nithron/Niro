// Package discord is Niro's Discord adapter, built on disgo. It translates
// gateway events and interactions into calls on the platform-agnostic core,
// and core results into Discord messages (Components V2).
package discord

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"net/url"
	"sync"
	"time"

	"github.com/disgoorg/disgo"
	"github.com/disgoorg/disgo/bot"
	"github.com/disgoorg/disgo/cache"
	"github.com/disgoorg/disgo/discord"
	"github.com/disgoorg/disgo/events"
	"github.com/disgoorg/disgo/gateway"
	"github.com/disgoorg/disgo/handler"
	"github.com/disgoorg/disgo/rest"
	"github.com/disgoorg/disgo/sharding"
	"github.com/disgoorg/snowflake/v2"
	"github.com/prometheus/client_golang/prometheus"

	"github.com/nithron/niro/internal/core"
	"github.com/nithron/niro/internal/i18n"
	"github.com/nithron/niro/internal/modules/guardian"
	"github.com/nithron/niro/internal/modules/logging"
	"github.com/nithron/niro/internal/modules/moderation"
	"github.com/nithron/niro/internal/modules/roles"
	"github.com/nithron/niro/internal/modules/tickets"
	"github.com/nithron/niro/internal/modules/welcome"
	"github.com/nithron/niro/internal/observability"
	"github.com/nithron/niro/internal/retention"
)

// Intents are the gateway intents the bot requests. Each milestone adds only
// what its features need (least privilege); see docs/plan.md §6.
//   - Guild Messages + Message Content (privileged): Guardian reads messages
//     for scam links and cross-channel spam; the server log shows edited and
//     deleted text. Message text is never stored in Postgres.
//   - Server Members (privileged): joins and leaves, for anti-raid, the age
//     gate and the server log.
//   - Guild Moderation: audit-log entries (who banned, kicked, changed what).
//   - Guild Voice States: voice joins, leaves and moves for the server log.
//   - Guild Invites: which invite a new member used.
//   - AutoMod Execution: AutoMod actions for the server log.
const Intents = gateway.IntentGuilds | gateway.IntentGuildMessages | gateway.IntentMessageContent |
	gateway.IntentGuildMembers | gateway.IntentGuildModeration | gateway.IntentGuildVoiceStates |
	gateway.IntentGuildInvites | gateway.IntentAutoModerationExecution

// privilegedIntents drops privileged intents the application is not allowed
// to use, so the bot still connects (with Guardian degraded) instead of
// being refused by Discord with "disallowed intents".
func privilegedIntents(token string, opts []rest.ClientConfigOpt, log *slog.Logger) gateway.Intents {
	intents := Intents
	app, err := rest.New(rest.NewClient(token, opts...)).GetCurrentApplication()
	if err != nil {
		log.Warn("could not check privileged intents; requesting all", "err", err)
		return intents
	}
	if app.Flags&(discord.ApplicationFlagGatewayMessageContent|discord.ApplicationFlagGatewayMessageContentLimited) == 0 {
		intents &^= gateway.IntentMessageContent
		log.Warn("Message Content intent is not enabled for this application: Guardian cannot check message text. Enable it in the Developer Portal → Bot.")
	}
	if app.Flags&(discord.ApplicationFlagGatewayGuildMembers|discord.ApplicationFlagGatewayGuildMemberLimited) == 0 {
		intents &^= gateway.IntentGuildMembers
		log.Warn("Server Members intent is not enabled for this application: anti-raid and the account-age gate are off. Enable it in the Developer Portal → Bot.")
	}
	return intents
}

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

	GuardianStore   guardian.Store
	GuardianTracker guardian.Tracker
	RolePanels      roles.Store
	Tickets         tickets.Store
	// LogCache keeps recent messages and invite snapshots for the server log.
	LogCache interface {
		logging.MessageCache
		logging.InviteSnapshots
	}
	// ScamFeedURLs are the scam-domain lists to download (https only).
	ScamFeedURLs []string
	// Lifecycle deletes old data (retention) and a server's data on request.
	Lifecycle Lifecycle
	// GuildCache forgets a server's Valkey keys when Niro leaves it.
	GuildCache GuildCache
	// Jobs records background job runs for alerts; may be nil.
	Jobs *observability.Jobs
	// RestURL replaces Discord's API base URL. Only the load test sets it,
	// to point the bot at a fake Discord (cmd/niro-soak).
	RestURL string
}

// Lifecycle is the store side of data retention.
type Lifecycle interface {
	retention.Store
	DeleteGuild(ctx context.Context, id snowflake.ID) (bool, error)
	// ReconcileGuilds marks active servers missing from present as left.
	ReconcileGuilds(ctx context.Context, present []snowflake.ID, maxShare float64) (int64, error)
}

// GuildCache holds per-server keys outside Postgres.
type GuildCache interface {
	ForgetGuild(ctx context.Context, id snowflake.ID) error
	// ForgetMessages drops cached message text for these channels.
	ForgetMessages(ctx context.Context, channels []snowflake.ID) error
}

// Bot is a running Niro Discord client.
type Bot struct {
	deps      Deps
	log       *slog.Logger
	client    *bot.Client
	emojis    *Emojis
	metrics   *metrics
	mod       *moderation.Service
	guard     *guardian.Service
	logs      *logging.Service
	roles     *roles.Service
	welcome   *welcome.Service
	tickets   *tickets.Service
	ruleNames ruleNameCache
	msgOrder  *orderedRunner
	blocklist *guardian.DomainSet
	pending   pendingBans
	// stopped is closed when Run returns; background work started by
	// handlers (the second erasure pass) watches it.
	stopped chan struct{}
	// present holds every server the gateway reported (READY, joins), for
	// the startup reconciliation.
	presentMu sync.Mutex
	present   map[snowflake.ID]bool
}

// New builds the client. Nothing connects until Run.
func New(deps Deps) (*Bot, error) {
	b := &Bot{deps: deps, log: deps.Log.With("component", "bot"), msgOrder: newOrderedRunner(1000, time.Minute), stopped: make(chan struct{}), present: map[snowflake.ID]bool{}}
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
	b.registerGuardian(router)
	b.registerRoles(router)
	b.registerTickets(router)
	router.Error(b.handleError)
	router.NotFound(func(e *handler.InteractionEvent) error {
		b.log.Warn("unhandled interaction", "type", e.Type())
		return nil
	})

	restOpts := []rest.ClientConfigOpt{rest.WithHTTPClient(&http.Client{Timeout: 20 * time.Second, Transport: b.metrics.restTransport(http.DefaultTransport)})}
	if deps.RestURL != "" {
		restOpts = append(restOpts, rest.WithURL(deps.RestURL))
	}
	b.client, err = disgo.New(deps.Token,
		bot.WithLogger(deps.Log.With("component", "disgo")),
		bot.WithShardManagerConfigOpts(
			sharding.WithAutoScaling(true), // reshard automatically when Discord asks
			sharding.WithGatewayConfigOpts(gateway.WithIntents(privilegedIntents(deps.Token, restOpts[1:], b.log))),
		),
		bot.WithCacheConfigOpts(cache.WithCaches(cache.FlagGuilds, cache.FlagRoles, cache.FlagChannels, cache.FlagMembers, cache.FlagVoiceStates)),
		bot.WithEventManagerConfigOpts(bot.WithGatewayHandlers(b.gatewayHandlers())),
		bot.WithRestClientConfigOpts(restOpts...),
		bot.WithEventListeners(router, bot.NewListenerFunc(b.metrics.countEvent), &events.ListenerAdapter{
			OnReady: func(e *events.Ready) {
				for _, g := range e.Guilds {
					b.notePresent(g.ID, true)
				}
			},
			OnGuildReady: func(e *events.GuildReady) { b.recordGuild(e.Guild); b.refreshInvites(e.GuildID) },
			OnGuildJoin: func(e *events.GuildJoin) {
				b.notePresent(e.GuildID, true)
				b.recordGuild(e.Guild)
				b.log.Info("joined guild", "guild_id", e.GuildID)
			},
			OnGuildLeave: func(e *events.GuildLeave) { b.notePresent(e.GuildID, false); b.forgetGuild(e.GuildID) },
			OnGuildMessageCreate: func(e *events.GuildMessageCreate) {
				b.onLogMessageCreate(e)
				b.onGuildMessage(e)
			},
			OnGuildMessageUpdate: b.onLogMessageUpdate,
			OnGuildMessageDelete: b.onLogMessageDelete,
			OnGuildMemberJoin: func(e *events.GuildMemberJoin) {
				b.onLogMemberJoin(e)
				b.onMemberJoin(e) // Guardian, then welcome
			},
			OnGuildMemberLeave: func(e *events.GuildMemberLeave) {
				b.onLogMemberLeave(e)
				b.onWelcomeLeave(e)
			},
			OnGuildAuditLogEntryCreate: b.onLogAudit,
			OnGuildVoiceJoin: func(e *events.GuildVoiceJoin) {
				b.onLogVoice(e.VoiceState.GuildID, e.Member, nil, e.VoiceState.ChannelID)
			},
			OnGuildVoiceMove: func(e *events.GuildVoiceMove) {
				b.onLogVoice(e.VoiceState.GuildID, e.Member, e.OldVoiceState.ChannelID, e.VoiceState.ChannelID)
			},
			OnGuildVoiceLeave: func(e *events.GuildVoiceLeave) {
				b.onLogVoice(e.VoiceState.GuildID, e.Member, e.OldVoiceState.ChannelID, nil)
			},
			OnAutoModerationActionExecution: b.onLogAutoMod,
			OnGuildInviteCreate: func(e *events.InviteCreate) {
				if e.GuildID != nil {
					b.refreshInvites(*e.GuildID)
				}
			},
			OnGuildInviteDelete: func(e *events.InviteDelete) {
				if e.GuildID != nil {
					b.refreshInvites(*e.GuildID)
				}
			},
		}),
	)
	if err != nil {
		return nil, fmt.Errorf("building discord client: %w", err)
	}
	b.emojis = NewEmojis(b.client.Rest, b.client.ApplicationID, b.log)
	b.roles = roles.NewService(roles.Deps{
		Store:    deps.RolePanels,
		Platform: rolesPlatform{client: b.client, p: b.presenter(), locale: b.guildLocale},
		Enabled: func(ctx context.Context, guildID snowflake.ID) (bool, error) {
			return deps.Config.IsEnabled(ctx, guildID, core.ModuleRoles)
		},
		VerifyGate: b.verifyGate,
	})
	b.tickets = tickets.NewService(tickets.Deps{
		Store:    deps.Tickets,
		Platform: ticketsPlatform{client: b.client, p: b.presenter(), locale: b.guildLocale},
		Settings: func(ctx context.Context, guildID snowflake.ID) (tickets.Settings, bool, error) {
			on, err := deps.Config.IsEnabled(ctx, guildID, core.ModuleTickets)
			if err != nil || !on {
				return tickets.Settings{}, false, err
			}
			s, err := tickets.LoadSettings(ctx, deps.Config, guildID)
			return s, err == nil, err
		},
		Log: b.log.With("module", "tickets"),
	})
	b.welcome = welcome.NewService(welcome.Deps{
		Settings: func(ctx context.Context, guildID snowflake.ID) (welcome.Settings, bool, error) {
			on, err := deps.Config.IsEnabled(ctx, guildID, core.ModuleWelcome)
			if err != nil || !on {
				return welcome.Settings{}, false, err
			}
			s, err := welcome.LoadSettings(ctx, deps.Config, guildID)
			return s, err == nil, err
		},
		Platform:    newWelcomePlatform(b.client, b.presenter(), b.guildLocale),
		Gate:        b.welcomeGate,
		RaidSummary: b.raidJoinSummary,
		Log:         b.log.With("module", "welcome"),
	})
	b.logs = logging.NewService(logging.Deps{
		Settings: func(ctx context.Context, guildID snowflake.ID) (logging.Settings, bool, error) {
			on, err := deps.Config.IsEnabled(ctx, guildID, core.ModuleLogging)
			if err != nil || !on {
				return logging.Settings{}, false, err
			}
			s, err := logging.LoadSettings(ctx, deps.Config, guildID)
			return s, err == nil, err
		},
		Cache: deps.LogCache, Snapshots: deps.LogCache, Invites: invitePlatform{client: b.client},
		Sink: newLogSink(b.client, b.presenter(), b.guildLocale, b.metrics, b.log.With("module", "logging")),
		ModLogChannel: func(ctx context.Context, guildID snowflake.ID) snowflake.ID {
			s, err := moderation.LoadSettings(ctx, deps.Config, guildID)
			if err != nil {
				return 0
			}
			return s.LogChannelID
		},
		GuardianAlertChannel: func(ctx context.Context, guildID snowflake.ID) snowflake.ID {
			return b.guard.AlertChannel(ctx, guildID)
		},
		Log: b.log.With("module", "logging"),
	})
	b.mod = moderation.NewService(deps.Cases, modPlatform{client: b.client, onDelete: b.markDeletes("purge")},
		modNotifier{client: b.client, p: b.presenter(), locale: b.guildLocale},
		func(ctx context.Context, guildID snowflake.ID) (moderation.Settings, error) {
			return moderation.LoadSettings(ctx, deps.Config, guildID)
		}, b.log.With("module", "moderation"))

	b.blocklist = guardian.NewDomainSet()
	b.guard = guardian.NewService(guardian.Deps{
		Store: deps.GuardianStore, Platform: newGuardianPlatform(b.client, b.markDeletes("guardian")),
		Alerts:  guardianAlerts{client: b.client, p: b.presenter(), locale: b.guildLocale},
		Tracker: deps.GuardianTracker, Moderation: b.mod, Blocklist: b.blocklist,
		Settings: func(ctx context.Context, guildID snowflake.ID) (guardian.Settings, bool, error) {
			on, err := deps.Config.IsEnabled(ctx, guildID, core.ModuleGuardian)
			if err != nil || !on {
				return guardian.Settings{}, false, err
			}
			s, err := guardian.LoadSettings(ctx, deps.Config, guildID)
			return s, err == nil, err
		},
		FallbackAlertChannel: func(ctx context.Context, guildID snowflake.ID) snowflake.ID {
			s, err := moderation.LoadSettings(ctx, deps.Config, guildID)
			if err != nil {
				return 0
			}
			return s.LogChannelID
		},
		OnEvent: b.logGuardianEvent,
		Log:     b.log.With("module", "guardian"),
	})
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
			b.mod.RunScheduler(ctx, actor, 30*time.Second, b.deps.Jobs.Reporter("scheduler"))
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
	defer close(b.stopped)
	b.log.Info("bot online", "application_id", b.client.ApplicationID, "version", b.deps.Version)
	b.deps.Jobs.Expect("scheduler", "guardian_purge", "transcript_purge", "scam_feed")
	go b.runScheduler(ctx)
	go b.guard.RunMaintenance(ctx, 15*time.Second, b.deps.Jobs.Reporter("guardian_purge"))
	go b.tickets.RunMaintenance(ctx, time.Hour, b.deps.Jobs.Reporter("transcript_purge"))
	go b.watchShards(ctx, 15*time.Second)
	if b.deps.Lifecycle != nil {
		b.deps.Jobs.Expect("retention")
		go b.runRetention(ctx)
	}
	feed := &guardian.FeedUpdater{Store: b.deps.GuardianStore, Set: b.blocklist, URLs: b.deps.ScamFeedURLs, Log: b.log.With("module", "guardian"), Done: b.deps.Jobs.Reporter("scam_feed")}
	go feed.Run(ctx)

	<-ctx.Done()
	closeCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	b.client.Close(closeCtx)
	b.log.Info("bot stopped")
	return nil
}

// Ready reports whether every shard is connected (readiness check). One
// shard down means a share of servers gets no moderation or protection, so
// it counts as not ready.
func (b *Bot) Ready(context.Context) error {
	total, down := 0, 0
	for s := range b.client.ShardManager.Shards() {
		total++
		if s.Status() != gateway.StatusReady {
			down++
		}
	}
	switch {
	case total == 0:
		return fmt.Errorf("no shard is running")
	case down > 0:
		return fmt.Errorf("%d of %d shards are not connected", down, total)
	}
	return nil
}

// runRetention starts the retention job once every shard is connected and
// the servers have had time to arrive: a server that re-added Niro while it
// was down is only marked present again when the gateway delivers it, and the
// purge must not run before that.
func (b *Bot) runRetention(ctx context.Context) {
	for b.Ready(ctx) != nil {
		select {
		case <-ctx.Done():
			return
		case <-time.After(10 * time.Second):
		}
	}
	b.reconcileGuilds(ctx)
	r := retention.Runner{Store: b.deps.Lifecycle, Log: b.log.With("component", "retention"), Done: b.deps.Jobs.Reporter("retention"), Delay: 5 * time.Minute}
	r.Run(ctx, time.Hour)
}

func (b *Bot) notePresent(id snowflake.ID, in bool) {
	b.presentMu.Lock()
	defer b.presentMu.Unlock()
	if in {
		b.present[id] = true
	} else {
		delete(b.present, id)
	}
}

// reconcileGuilds marks servers that removed Niro while it was offline as
// left, so their data is purged after the grace period like any other. It
// runs once every shard has reported its servers.
func (b *Bot) reconcileGuilds(ctx context.Context) {
	b.presentMu.Lock()
	ids := make([]snowflake.ID, 0, len(b.present))
	for id := range b.present {
		ids = append(ids, id)
	}
	b.presentMu.Unlock()
	n, err := b.deps.Lifecycle.ReconcileGuilds(ctx, ids, 0.1)
	if err != nil {
		b.log.Error("reconciling servers failed", "err", err)
		b.deps.Jobs.Done("reconcile", err)
		return
	}
	b.deps.Jobs.Done("reconcile", nil)
	if n > 0 {
		b.log.Info("servers that removed Niro while it was offline marked as left", "count", n)
	}
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
	// Only a real removal arrives here: disgo reports an outage as
	// GuildUnavailable instead. The data stays for the 30-day grace period
	// (retention.LeftGuildGrace); the cached keys go now.
	if err := b.deps.Guilds.MarkGuildLeft(ctx, id); err != nil {
		b.log.Error("marking guild as left failed", "guild_id", id, "err", err)
	}
	if b.deps.GuildCache != nil {
		if err := b.deps.GuildCache.ForgetGuild(ctx, id); err != nil {
			b.log.Warn("clearing the server's cached keys failed", "guild_id", id, "err", err)
		}
	}
	b.metrics.guilds.Set(float64(b.client.Caches.GuildsLen()))
	b.log.Info("left guild", "guild_id", id)
}
