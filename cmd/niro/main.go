// Command niro runs the Niro Discord bot and/or its dashboard API.
//
//	niro all       bot + API + dashboard in one process (self-hosting, development)
//	niro bot       Discord gateway only
//	niro api       dashboard + API only
//	niro migrate   apply database migrations and exit
//	niro version   print the version
//
// Configuration comes from NIRO_* environment variables; in development a
// .env file in the working directory is loaded too (see .env.example).
package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/joho/godotenv"
	"golang.org/x/sync/errgroup"

	"github.com/nithron/niro/internal/api"
	"github.com/nithron/niro/internal/buildinfo"
	"github.com/nithron/niro/internal/cache"
	"github.com/nithron/niro/internal/config"
	"github.com/nithron/niro/internal/core"
	"github.com/nithron/niro/internal/i18n"
	"github.com/nithron/niro/internal/modules/guardian"
	"github.com/nithron/niro/internal/modules/logging"
	"github.com/nithron/niro/internal/modules/moderation"
	"github.com/nithron/niro/internal/observability"
	"github.com/nithron/niro/internal/platform/discord"
	"github.com/nithron/niro/internal/rpc"
	"github.com/nithron/niro/internal/store"
	"github.com/nithron/niro/web"
)

const usage = `Usage: niro <command>

Commands:
  all       run the bot, API and dashboard in one process
  bot       run the Discord bot only
  api       run the dashboard and API only
  migrate   apply database migrations and exit
  version   print the version and exit
`

func main() {
	// .env is a development convenience; real environment variables win.
	if err := godotenv.Load(); err != nil && !errors.Is(err, os.ErrNotExist) {
		fmt.Fprintln(os.Stderr, "reading .env:", err)
		os.Exit(1)
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	os.Exit(run(ctx, os.Args[1:], os.Getenv, os.Stdout, os.Stderr))
}

func run(ctx context.Context, args []string, getenv func(string) string, stdout, stderr io.Writer) int {
	cmd := ""
	if len(args) > 0 {
		cmd = args[0]
	}
	role := config.Role{}
	switch cmd {
	case "version":
		fmt.Fprintln(stdout, buildinfo.String())
		return 0
	case "all":
		role = config.Role{Bot: true, API: true}
	case "bot":
		role.Bot = true
	case "api":
		role.API = true
	case "migrate":
	default:
		fmt.Fprint(stderr, usage)
		return 2
	}

	cfg, err := config.Load(getenv)
	if err == nil {
		err = cfg.Validate(role)
	}
	if err != nil {
		fmt.Fprintf(stderr, "configuration error:\n%v\n\nSee .env.example and docs/dev-setup.md.\n", err)
		return 2
	}
	log := observability.NewLogger(stderr, cfg.LogLevel, cfg.LogFormat).With("version", buildinfo.String())

	if err := start(ctx, cmd, role, cfg, log); err != nil {
		log.Error("niro stopped with an error", "err", err)
		return 1
	}
	return 0
}

func start(ctx context.Context, cmd string, role config.Role, cfg config.Config, log *slog.Logger) error {
	pool, err := store.Connect(ctx, cfg.DatabaseURL, 30*time.Second)
	if err != nil {
		return err
	}
	defer pool.Close()

	// `all` migrates on start so self-hosters need one command; the hosted
	// deployment runs `niro migrate` as a separate release step.
	if cmd == "migrate" || cmd == "all" {
		results, err := store.Migrate(ctx, pool)
		if err != nil {
			return fmt.Errorf("migrating: %w", err)
		}
		for _, r := range results {
			log.Info("migration applied", "migration", r.Source.Version, "file", r.Source.Path, "duration", r.Duration)
		}
		if cmd == "migrate" {
			log.Info("database is up to date")
			return nil
		}
	}

	kv, err := cache.Connect(ctx, cfg.ValkeyAddr, cfg.ValkeyPassword, 30*time.Second)
	if err != nil {
		return err
	}
	defer kv.Close()

	db := store.New(pool)
	modules := core.WithSettings(core.DefaultModules(), core.ModuleModeration, moderation.SettingsSpec{})
	modules = core.WithSettings(modules, core.ModuleGuardian, guardian.SettingsSpec{})
	modules = core.WithSettings(modules, core.ModuleLogging, logging.SettingsSpec{})
	registry, err := core.NewRegistry(modules...)
	if err != nil {
		return err
	}
	configs, err := core.NewConfigService(registry, db, kv, log)
	if err != nil {
		return err
	}
	catalog, err := i18n.Load()
	if err != nil {
		return err
	}

	metrics := observability.NewRegistry()
	checks := observability.NewChecks()
	checks.Add("postgres", db.Ping)
	checks.Add("valkey", kv.Ping)

	g, ctx := errgroup.WithContext(ctx)

	g.Go(func() error {
		configs.RunInvalidation(ctx, func(c core.ConfigChange) {
			log.Info("config change received", "guild_id", c.GuildID, "module", c.Module, "version", c.Version)
		})
		return nil
	})

	g.Go(func() error {
		return serveHTTP(ctx, log, "ops", cfg.OpsAddr, observability.OpsHandler(metrics, checks))
	})

	if role.Bot {
		b, err := discord.New(discord.Deps{
			Token: cfg.Discord.Token, DevGuildID: cfg.Discord.DevGuildID, DashboardURL: cfg.DashboardURL,
			Version: buildinfo.String(), Log: log, Config: configs, Guilds: db, Cases: db, Catalog: catalog, Metrics: metrics,
			GuardianStore: db, GuardianTracker: kv.Guardian(), ScamFeedURLs: scamFeeds(cfg),
		})
		if err != nil {
			return err
		}
		checks.Add("discord", b.Ready)
		g.Go(func() error { return b.Run(ctx) })

		host, _ := os.Hostname()
		rpcServer := rpc.NewServer(kv.Raw(), fmt.Sprintf("%s-%d", host, os.Getpid()), log)
		b.RegisterRPC(rpcServer)
		g.Go(func() error { return rpcServer.Run(ctx) })
	}

	if role.API {
		handler, err := api.New(api.Deps{
			Config: api.Config{
				PublicURL: cfg.PublicURL, DashboardURL: cfg.DashboardURL,
				SecureCookies: cfg.SecureCookies(), ClientID: cfg.Discord.ClientID,
			},
			Log:       log,
			OAuth:     api.NewDiscordOAuth(cfg.Discord.ClientID, cfg.Discord.ClientSecret, cfg.OAuthRedirectURI()),
			KV:        kv,
			Guilds:    db,
			Configs:   configs,
			Cases:     db,
			Guardian:  db,
			RPC:       rpc.NewClient(kv.Raw(), 10*time.Second),
			Dashboard: web.Dist(),
			Metrics:   metrics,
		})
		if err != nil {
			return err
		}
		log.Info("dashboard available", "url", cfg.PublicURL.String())
		g.Go(func() error { return serveHTTP(ctx, log, "api", cfg.HTTPAddr, handler) })
	}

	return g.Wait()
}

// scamFeeds returns the configured scam-domain feeds, or Niro's default.
func scamFeeds(cfg config.Config) []string {
	if cfg.ScamFeeds == nil {
		return []string{guardian.DefaultFeedURL}
	}
	return cfg.ScamFeeds
}

// serveHTTP runs an HTTP server until ctx is done, then shuts it down gracefully.
func serveHTTP(ctx context.Context, log *slog.Logger, name, addr string, h http.Handler) error {
	srv := &http.Server{
		Addr: addr, Handler: h,
		ReadHeaderTimeout: 10 * time.Second, ReadTimeout: 30 * time.Second,
		WriteTimeout: 30 * time.Second, IdleTimeout: 2 * time.Minute,
	}
	errc := make(chan error, 1)
	go func() {
		log.Info("http server listening", "server", name, "addr", addr)
		if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			errc <- fmt.Errorf("%s server: %w", name, err)
		}
		close(errc)
	}()
	select {
	case err := <-errc:
		return err
	case <-ctx.Done():
	}
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	return srv.Shutdown(shutdownCtx)
}
