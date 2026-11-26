// Command niro-soak load-tests Niro's bot. It runs the real event handlers,
// with real Postgres and Valkey, against synthetic gateway traffic and a fake
// Discord REST API on localhost, and checks that memory, goroutines and
// handler latency hold steady.
//
//	go run ./cmd/niro-soak -database postgres://niro:niro@localhost:5432/niro_soak?sslmode=disable \
//	    -guilds 2500 -rate 100 -duration 2h
//
// Use a throwaway database: the run creates thousands of servers in it. It
// refuses to start on a database that already has servers unless -reuse is set.
// Results: docs/ops/baselines.md.
package main

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"log/slog"
	"math/rand/v2"
	"net"
	"net/http"
	"net/url"
	"os"
	"os/signal"
	"runtime"
	"slices"
	"sync"
	"sync/atomic"
	"syscall"
	"time"

	"github.com/disgoorg/disgo/discord"
	"github.com/disgoorg/disgo/gateway"
	"github.com/disgoorg/snowflake/v2"
	"github.com/prometheus/client_golang/prometheus"

	"github.com/nithron/niro/internal/cache"
	"github.com/nithron/niro/internal/core"
	"github.com/nithron/niro/internal/i18n"
	"github.com/nithron/niro/internal/modules/guardian"
	"github.com/nithron/niro/internal/modules/logging"
	"github.com/nithron/niro/internal/modules/moderation"
	"github.com/nithron/niro/internal/modules/tickets"
	"github.com/nithron/niro/internal/modules/welcome"
	dc "github.com/nithron/niro/internal/platform/discord"
	"github.com/nithron/niro/internal/store"
)

type options struct {
	database, valkey, valkeyPassword, out     string
	guilds, channels, members, rate, raidSize int
	duration, warmup, raidEvery, report       time.Duration
	maxP99                                    time.Duration
	reuse                                     bool
}

func main() {
	var o options
	flag.StringVar(&o.database, "database", "", "Postgres URL of a throwaway database (required)")
	flag.StringVar(&o.valkey, "valkey", "localhost:6379", "Valkey address")
	flag.StringVar(&o.valkeyPassword, "valkey-password", "", "Valkey password")
	flag.IntVar(&o.guilds, "guilds", 2500, "synthetic servers")
	flag.IntVar(&o.channels, "channels", 5, "text channels per server")
	flag.IntVar(&o.members, "members", 20, "members per server at the start")
	flag.IntVar(&o.rate, "rate", 100, "messages a second, across all servers")
	flag.DurationVar(&o.duration, "duration", 2*time.Hour, "how long to send traffic")
	flag.DurationVar(&o.warmup, "warmup", 2*time.Minute, "traffic before the memory and goroutine baseline is taken")
	flag.DurationVar(&o.raidEvery, "raid-every", 10*time.Minute, "time between raid bursts")
	flag.IntVar(&o.raidSize, "raid-size", 50, "joins in a raid burst")
	flag.DurationVar(&o.report, "report", 30*time.Second, "progress report interval")
	flag.DurationVar(&o.maxP99, "max-p99", 50*time.Millisecond, "fail if the p99 handler time is above this")
	flag.StringVar(&o.out, "out", "", "write the results as JSON to this file")
	flag.BoolVar(&o.reuse, "reuse", false, "allow a database that already has servers")
	flag.Parse()
	if o.database == "" {
		fmt.Fprintln(os.Stderr, "niro-soak: -database is required (use a throwaway database)")
		os.Exit(2)
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	res, err := run(ctx, o)
	if err != nil {
		fmt.Fprintln(os.Stderr, "niro-soak:", err)
		os.Exit(1)
	}
	res.print(os.Stdout)
	if o.out != "" {
		b, _ := json.MarshalIndent(res, "", "  ")
		if err := os.WriteFile(o.out, b, 0o644); err != nil {
			fmt.Fprintln(os.Stderr, "niro-soak:", err)
		}
	}
	if !res.Pass {
		os.Exit(1)
	}
}

// ---- results -----------------------------------------------------------------

type result struct {
	Guilds, Rate          int
	Duration              string
	Events                int64
	EventsPerSecond       float64
	P50, P99, Max         string
	HeapWarmMB, HeapEndMB float64
	GoroutinesWarm        int
	GoroutinesEnd         int
	CPUCores              float64
	RESTRequests          int64
	Warnings, Errors      int64
	Failures              []string
	Pass                  bool
}

func (r result) print(w io.Writer) {
	fmt.Fprintf(w, "\n== niro-soak: %d servers, %d msg/s, %s ==\n", r.Guilds, r.Rate, r.Duration)
	fmt.Fprintf(w, "events        %d (%.0f/s)\n", r.Events, r.EventsPerSecond)
	fmt.Fprintf(w, "handler time  p50 %s, p99 %s, max %s\n", r.P50, r.P99, r.Max)
	fmt.Fprintf(w, "heap          %.1f MB after warm-up, %.1f MB at the end (after GC)\n", r.HeapWarmMB, r.HeapEndMB)
	fmt.Fprintf(w, "goroutines    %d after warm-up, %d at the end\n", r.GoroutinesWarm, r.GoroutinesEnd)
	fmt.Fprintf(w, "cpu           %.2f cores on average\n", r.CPUCores)
	fmt.Fprintf(w, "fake REST     %d requests\n", r.RESTRequests)
	fmt.Fprintf(w, "log           %d warnings, %d errors\n", r.Warnings, r.Errors)
	if r.Pass {
		fmt.Fprintln(w, "PASS")
		return
	}
	for _, f := range r.Failures {
		fmt.Fprintln(w, "FAIL:", f)
	}
}

// ---- the run -----------------------------------------------------------------

func run(ctx context.Context, o options) (result, error) {
	logs := &countingHandler{}
	log := slog.New(logs)

	pool, err := store.Connect(ctx, o.database, 30*time.Second)
	if err != nil {
		return result{}, err
	}
	defer pool.Close()
	if _, err := store.Migrate(ctx, pool); err != nil {
		return result{}, fmt.Errorf("migrating: %w", err)
	}
	var existing int
	if err := pool.QueryRow(ctx, "SELECT count(*) FROM guilds").Scan(&existing); err != nil {
		return result{}, err
	}
	if existing > 0 && !o.reuse {
		return result{}, fmt.Errorf("the database already has %d servers; use a throwaway database or pass -reuse", existing)
	}
	kv, err := cache.Connect(ctx, o.valkey, o.valkeyPassword, 30*time.Second)
	if err != nil {
		return result{}, err
	}
	defer kv.Close()

	fake := newFakeDiscord()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return result{}, err
	}
	srv := &http.Server{Handler: fake, ReadHeaderTimeout: 5 * time.Second}
	go func() { _ = srv.Serve(ln) }()
	defer func() { _ = srv.Close() }()

	db := store.New(pool)
	modules := core.WithSettings(core.DefaultModules(), core.ModuleModeration, moderation.SettingsSpec{})
	modules = core.WithSettings(modules, core.ModuleGuardian, guardian.SettingsSpec{})
	modules = core.WithSettings(modules, core.ModuleLogging, logging.SettingsSpec{})
	modules = core.WithSettings(modules, core.ModuleWelcome, welcome.SettingsSpec{})
	modules = core.WithSettings(modules, core.ModuleTickets, tickets.SettingsSpec{})
	registry, err := core.NewRegistry(modules...)
	if err != nil {
		return result{}, err
	}
	configs, err := core.NewConfigService(registry, db, kv, log)
	if err != nil {
		return result{}, err
	}
	catalog, err := i18n.Load()
	if err != nil {
		return result{}, err
	}
	dash, _ := url.Parse("http://localhost:8080")
	b, err := dc.New(dc.Deps{
		Token: fakeToken(fake.appID), DashboardURL: dash, Version: "soak", Log: log,
		Config: configs, Guilds: db, Cases: db, Catalog: catalog, Metrics: prometheus.NewRegistry(),
		GuardianStore: db, GuardianTracker: kv.Guardian(), LogCache: kv.Logging(), RolePanels: db, Tickets: db,
		Lifecycle: db, GuildCache: kv, RestURL: "http://" + ln.Addr().String() + "/api/v10",
	})
	if err != nil {
		return result{}, err
	}

	w := newWorld(o, fake.appID)
	fmt.Printf("setting up %d servers…\n", o.guilds)
	if err := w.setup(ctx, b, configs); err != nil {
		return result{}, err
	}

	start := time.Now()
	cpuStart := cpuTime()
	stats := &latencies{}
	var events atomic.Int64
	inject := func(t gateway.EventType, data gateway.EventData) {
		t0 := time.Now()
		b.Inject(t, int(events.Add(1)), data)
		stats.add(time.Since(t0))
	}

	var heapWarm float64
	var goroutinesWarm int
	warmDone := make(chan struct{})
	go func() {
		select {
		case <-time.After(o.warmup):
		case <-ctx.Done():
		}
		heapWarm, goroutinesWarm = heapAfterGC(), runtime.NumGoroutine()
		fmt.Printf("baseline after warm-up: heap %.1f MB, %d goroutines\n", heapWarm, goroutinesWarm)
		close(warmDone)
	}()

	loadCtx, cancel := context.WithTimeout(ctx, o.duration)
	defer cancel()
	go w.reportLoop(loadCtx, o.report, start, stats, &events, fake, logs)
	w.traffic(loadCtx, inject)
	<-warmDone

	// Let asynchronous work (log posts, ordered runners) drain.
	time.Sleep(10 * time.Second)
	elapsed := time.Since(start)
	res := result{
		Guilds: o.guilds, Rate: o.rate, Duration: elapsed.Round(time.Second).String(),
		Events: events.Load(), EventsPerSecond: float64(events.Load()) / elapsed.Seconds(),
		HeapWarmMB: heapWarm, HeapEndMB: heapAfterGC(), GoroutinesWarm: goroutinesWarm, GoroutinesEnd: runtime.NumGoroutine(),
		CPUCores: (cpuTime() - cpuStart).Seconds() / elapsed.Seconds(), RESTRequests: fake.requests.Load(),
		Warnings: logs.warns.Load(), Errors: logs.errors.Load(),
	}
	p50, p99, mx := stats.overall()
	res.P50, res.P99, res.Max = p50.String(), p99.String(), mx.String()
	if res.Errors > 0 {
		res.Failures = append(res.Failures, fmt.Sprintf("%d errors were logged", res.Errors))
	}
	if p99 > o.maxP99 {
		res.Failures = append(res.Failures, fmt.Sprintf("p99 handler time %s is above %s", p99, o.maxP99))
	}
	if res.HeapEndMB > res.HeapWarmMB*1.3+32 {
		res.Failures = append(res.Failures, fmt.Sprintf("heap grew from %.1f MB to %.1f MB", res.HeapWarmMB, res.HeapEndMB))
	}
	if float64(res.GoroutinesEnd) > float64(res.GoroutinesWarm)*1.2+50 {
		res.Failures = append(res.Failures, fmt.Sprintf("goroutines grew from %d to %d", res.GoroutinesWarm, res.GoroutinesEnd))
	}
	res.Pass = len(res.Failures) == 0 && ctx.Err() == nil
	if ctx.Err() != nil {
		res.Failures = append(res.Failures, "interrupted")
	}
	return res, nil
}

func heapAfterGC() float64 {
	runtime.GC()
	var m runtime.MemStats
	runtime.ReadMemStats(&m)
	return float64(m.HeapAlloc) / (1 << 20)
}

func cpuTime() time.Duration {
	var ru syscall.Rusage
	if err := syscall.Getrusage(syscall.RUSAGE_SELF, &ru); err != nil {
		return 0
	}
	return time.Duration(ru.Utime.Nano() + ru.Stime.Nano())
}

// fakeToken builds a token disgo accepts: its first part is the application
// ID in base64. It is not a real credential.
func fakeToken(app snowflake.ID) string {
	return base64.RawStdEncoding.EncodeToString([]byte(app.String())) + ".soak.soak"
}

// ---- synthetic servers and traffic ------------------------------------------

type guildSim struct {
	id       snowflake.ID
	channels []snowflake.ID
	members  []snowflake.ID
}

type world struct {
	o      options
	bot    snowflake.ID
	guilds []guildSim
	seq    atomic.Uint64
	rng    *rand.Rand
	recent []sentMessage // ring of recent messages for edits and deletes
	next   int
}

type sentMessage struct{ guild, channel, id, author snowflake.ID }

func newWorld(o options, bot snowflake.ID) *world {
	return &world{o: o, bot: bot, rng: rand.New(rand.NewPCG(1, 2)), recent: make([]sentMessage, 4096)}
}

// id returns a unique snowflake whose timestamp is t (account and message age).
func (w *world) id(t time.Time) snowflake.ID {
	return snowflake.ID(uint64(snowflake.New(t)) | (w.seq.Add(1) & 0x3FFFFF))
}

func (w *world) setup(ctx context.Context, b *dc.Bot, configs *core.ConfigService) error {
	old := time.Now().AddDate(-3, 0, 0)
	ready := map[string]any{"v": 10, "user": map[string]any{"id": w.bot, "username": "Niro", "discriminator": "0", "bot": true},
		"session_id": "soak", "resume_gateway_url": "wss://localhost", "guilds": []any{}}
	w.guilds = make([]guildSim, w.o.guilds)
	var unavailable []any
	for i := range w.guilds {
		g := guildSim{id: w.id(old)}
		for range w.o.channels {
			g.channels = append(g.channels, w.id(old))
		}
		for range w.o.members {
			g.members = append(g.members, w.id(old))
		}
		w.guilds[i] = g
		unavailable = append(unavailable, map[string]any{"id": g.id, "unavailable": true})
	}
	ready["guilds"] = unavailable
	var ev gateway.EventReady
	if err := remarshal(ready, &ev); err != nil {
		return fmt.Errorf("ready: %w", err)
	}
	b.Inject(gateway.EventTypeReady, 0, ev)

	actor := w.bot
	for i, g := range w.guilds {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		var gc gateway.EventGuildCreate
		if err := remarshal(w.guildJSON(g), &gc); err != nil {
			return fmt.Errorf("guild create: %w", err)
		}
		b.Inject(gateway.EventTypeGuildCreate, 0, gc)
		log := g.channels[0]
		welcomeDoc := map[string]any{"version": 1, "blocks": []any{map[string]any{"type": "text", "text": "Welcome {user} to {server}!"}}}
		for _, m := range []core.ModuleID{core.ModuleGuardian, core.ModuleLogging, core.ModuleWelcome} {
			if _, err := configs.SetEnabled(ctx, g.id, m, true, &actor, core.SourceDashboard); err != nil {
				return fmt.Errorf("enabling %s: %w", m, err)
			}
		}
		settings := map[core.ModuleID]any{
			core.ModuleLogging: map[string]any{"default_channel_id": log},
			core.ModuleWelcome: map[string]any{"welcome_enabled": true, "welcome_channel_id": g.channels[1%len(g.channels)], "welcome_message": welcomeDoc,
				"goodbye_enabled": true, "goodbye_channel_id": g.channels[1%len(g.channels)], "goodbye_message": welcomeDoc},
			core.ModuleGuardian: map[string]any{"alert_channel_id": log},
		}
		for m, s := range settings {
			raw, _ := json.Marshal(s)
			if _, err := configs.SetSettings(ctx, g.id, m, raw, &actor, core.SourceDashboard); err != nil {
				return fmt.Errorf("settings for %s: %w", m, err)
			}
		}
		if (i+1)%500 == 0 {
			fmt.Printf("  %d servers ready\n", i+1)
		}
	}
	return nil
}

func (w *world) guildJSON(g guildSim) map[string]any {
	old := "2023-01-01T00:00:00Z"
	botRole := w.id(time.Now())
	botPerms := discord.PermissionViewChannel | discord.PermissionSendMessages | discord.PermissionEmbedLinks |
		discord.PermissionAttachFiles | discord.PermissionReadMessageHistory | discord.PermissionManageMessages |
		discord.PermissionModerateMembers | discord.PermissionKickMembers | discord.PermissionBanMembers |
		discord.PermissionManageRoles | discord.PermissionManageChannels | discord.PermissionViewAuditLog
	roles := []any{
		map[string]any{"id": g.id, "name": "@everyone", "permissions": discord.PermissionViewChannel | discord.PermissionSendMessages, "position": 0},
		map[string]any{"id": botRole, "name": "Niro", "permissions": botPerms, "position": 10, "managed": true},
	}
	var channels []any
	for i, c := range g.channels {
		channels = append(channels, map[string]any{"id": c, "type": 0, "name": fmt.Sprintf("channel-%d", i), "position": i, "guild_id": g.id, "permission_overwrites": []any{}})
	}
	members := []any{map[string]any{"user": map[string]any{"id": w.bot, "username": "Niro", "bot": true}, "roles": []any{botRole}, "joined_at": old}}
	for i, m := range g.members {
		members = append(members, map[string]any{"user": map[string]any{"id": m, "username": fmt.Sprintf("member%d", i)}, "roles": []any{}, "joined_at": old})
	}
	return map[string]any{
		"id": g.id, "name": "Soak " + g.id.String(), "owner_id": g.members[0], "roles": roles, "channels": channels,
		"members": members, "member_count": len(members), "joined_at": old, "preferred_locale": "en-US",
		"features": []any{}, "emojis": []any{}, "stickers": []any{}, "voice_states": []any{}, "threads": []any{},
		"presences": []any{}, "stage_instances": []any{}, "guild_scheduled_events": []any{}, "soundboard_sounds": []any{},
	}
}

var contents = []string{
	"hello everyone", "anyone up for a game tonight?", "lol", "see the pinned post",
	"check this out https://example.com/article", "docs: https://docs.example.org/page",
	"free nitro here https://dlscord-gift.com/claim", "gg <@%s>",
}

func (w *world) message(g guildSim) sentMessage {
	return sentMessage{guild: g.id, channel: g.channels[w.rng.IntN(len(g.channels))], id: w.id(time.Now()), author: g.members[w.rng.IntN(len(g.members))]}
}

func (w *world) messageJSON(m sentMessage, content string, edited bool) map[string]any {
	now := time.Now().UTC().Format(time.RFC3339Nano)
	msg := map[string]any{
		"id": m.id, "channel_id": m.channel, "guild_id": m.guild, "content": content, "timestamp": now, "type": 0,
		"author": map[string]any{"id": m.author, "username": "member"}, "member": map[string]any{"roles": []any{}, "joined_at": "2023-01-01T00:00:00Z"},
		"mentions": []any{}, "attachments": []any{}, "embeds": []any{}, "mention_roles": []any{},
	}
	if edited {
		msg["edited_timestamp"] = now
	}
	return msg
}

func (w *world) memberJSON(g, user snowflake.ID) map[string]any {
	return map[string]any{"guild_id": g, "user": map[string]any{"id": user, "username": "newcomer"}, "roles": []any{}, "joined_at": time.Now().UTC().Format(time.RFC3339)}
}

// traffic sends messages at the configured rate, with edits, deletes, joins,
// leaves and periodic raid bursts, until ctx ends.
func (w *world) traffic(ctx context.Context, inject func(gateway.EventType, gateway.EventData)) {
	tick := time.NewTicker(10 * time.Millisecond)
	defer tick.Stop()
	raid := time.NewTicker(w.o.raidEvery)
	defer raid.Stop()
	perTick := float64(w.o.rate) / 100
	var carry float64
	for {
		select {
		case <-ctx.Done():
			return
		case <-raid.C:
			g := w.guilds[w.rng.IntN(len(w.guilds))]
			for range w.o.raidSize {
				var ev gateway.EventGuildMemberAdd
				if remarshal(w.memberJSON(g.id, w.id(time.Now().Add(-time.Hour))), &ev) == nil {
					inject(gateway.EventTypeGuildMemberAdd, ev)
				}
			}
		case <-tick.C:
			carry += perTick
			for ; carry >= 1; carry-- {
				w.one(inject)
			}
		}
	}
}

func (w *world) one(inject func(gateway.EventType, gateway.EventData)) {
	g := w.guilds[w.rng.IntN(len(w.guilds))]
	switch r := w.rng.Float64(); {
	case r < 0.08: // edit a recent message
		if m := w.recent[w.rng.IntN(len(w.recent))]; m.id != 0 {
			var ev gateway.EventMessageUpdate
			if remarshal(w.messageJSON(m, "edited: "+contents[w.rng.IntN(4)], true), &ev) == nil {
				inject(gateway.EventTypeMessageUpdate, ev)
			}
			return
		}
	case r < 0.12: // delete a recent message
		if m := w.recent[w.rng.IntN(len(w.recent))]; m.id != 0 {
			gid := m.guild
			inject(gateway.EventTypeMessageDelete, gateway.EventMessageDelete{ID: m.id, ChannelID: m.channel, GuildID: &gid})
			return
		}
	case r < 0.13: // someone joins
		var ev gateway.EventGuildMemberAdd
		if remarshal(w.memberJSON(g.id, w.id(time.Now().AddDate(0, -w.rng.IntN(36), 0))), &ev) == nil {
			inject(gateway.EventTypeGuildMemberAdd, ev)
		}
		return
	case r < 0.135: // someone leaves
		inject(gateway.EventTypeGuildMemberRemove, gateway.EventGuildMemberRemove{GuildID: g.id, User: discord.User{ID: w.id(time.Now()), Username: "leaver"}})
		return
	}
	m := w.message(g)
	content := contents[w.rng.IntN(len(contents))]
	if content == "gg <@%s>" {
		content = fmt.Sprintf(content, g.members[0])
	}
	var ev gateway.EventMessageCreate
	if remarshal(w.messageJSON(m, content, false), &ev) == nil {
		inject(gateway.EventTypeMessageCreate, ev)
		w.recent[w.next] = m
		w.next = (w.next + 1) % len(w.recent)
	}
}

func (w *world) reportLoop(ctx context.Context, every time.Duration, start time.Time, stats *latencies, events *atomic.Int64, fake *fakeDiscord, logs *countingHandler) {
	t := time.NewTicker(every)
	defer t.Stop()
	var lastEvents int64
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
		}
		n := events.Load()
		p50, p99 := stats.window()
		var m runtime.MemStats
		runtime.ReadMemStats(&m)
		fmt.Printf("%8s  %6.0f ev/s  p50 %-9s p99 %-9s heap %6.1f MB  goroutines %5d  REST %7d  warn %d  err %d\n",
			time.Since(start).Round(time.Second), float64(n-lastEvents)/every.Seconds(), p50, p99,
			float64(m.HeapAlloc)/(1<<20), runtime.NumGoroutine(), fake.requests.Load(), logs.warns.Load(), logs.errors.Load())
		lastEvents = n
	}
}

func remarshal(in any, out any) error {
	b, err := json.Marshal(in)
	if err != nil {
		return err
	}
	return json.Unmarshal(b, out)
}

// ---- latency stats -------------------------------------------------------------

type latencies struct {
	mu      sync.Mutex
	win     []time.Duration
	samples []time.Duration // reservoir for the overall percentiles
	n       int64
	max     time.Duration
	rng     *rand.Rand
}

const reservoir = 200_000

func (l *latencies) add(d time.Duration) {
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.rng == nil {
		l.rng = rand.New(rand.NewPCG(3, 4))
	}
	l.n++
	l.win = append(l.win, d)
	l.max = max(l.max, d)
	if len(l.samples) < reservoir {
		l.samples = append(l.samples, d)
	} else if j := l.rng.Int64N(l.n); j < reservoir {
		l.samples[j] = d
	}
}

func pct(s []time.Duration, p float64) time.Duration {
	if len(s) == 0 {
		return 0
	}
	slices.Sort(s)
	return s[min(len(s)-1, int(float64(len(s))*p))]
}

func (l *latencies) window() (p50, p99 time.Duration) {
	l.mu.Lock()
	w := l.win
	l.win = nil
	l.mu.Unlock()
	return pct(w, .5).Round(time.Microsecond), pct(w, .99).Round(time.Microsecond)
}

func (l *latencies) overall() (p50, p99, mx time.Duration) {
	l.mu.Lock()
	s := slices.Clone(l.samples)
	mx = l.max
	l.mu.Unlock()
	return pct(s, .5).Round(time.Microsecond), pct(s, .99).Round(time.Microsecond), mx.Round(time.Microsecond)
}

// ---- logging -------------------------------------------------------------------

// countingHandler counts warnings and errors and prints the first few of each.
type countingHandler struct {
	warns, errors atomic.Int64
	attrs         []slog.Attr
}

func (h *countingHandler) Enabled(_ context.Context, l slog.Level) bool { return l >= slog.LevelWarn }

func (h *countingHandler) Handle(_ context.Context, r slog.Record) error {
	var n int64
	if r.Level >= slog.LevelError {
		n = h.errors.Add(1)
	} else {
		n = h.warns.Add(1)
	}
	if n <= 10 {
		attrs := ""
		r.Attrs(func(a slog.Attr) bool { attrs += " " + a.String(); return true })
		for _, a := range h.attrs {
			attrs += " " + a.String()
		}
		fmt.Fprintf(os.Stderr, "[%s] %s%s\n", r.Level, r.Message, attrs)
	}
	return nil
}

func (h *countingHandler) WithAttrs(as []slog.Attr) slog.Handler {
	return &derived{root: h, attrs: append(slices.Clone(h.attrs), as...)}
}

func (h *countingHandler) WithGroup(string) slog.Handler { return h }

type derived struct {
	root  *countingHandler
	attrs []slog.Attr
}

func (d *derived) Enabled(ctx context.Context, l slog.Level) bool { return d.root.Enabled(ctx, l) }
func (d *derived) Handle(ctx context.Context, r slog.Record) error {
	r.AddAttrs(d.attrs...)
	return d.root.Handle(ctx, r)
}
func (d *derived) WithAttrs(as []slog.Attr) slog.Handler {
	return &derived{root: d.root, attrs: append(slices.Clone(d.attrs), as...)}
}
func (d *derived) WithGroup(string) slog.Handler { return d }
