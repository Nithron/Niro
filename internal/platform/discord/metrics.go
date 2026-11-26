package discord

import (
	"context"
	"fmt"
	"net/http"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/disgoorg/disgo/bot"
	"github.com/disgoorg/disgo/discord"
	"github.com/disgoorg/disgo/gateway"
	"github.com/disgoorg/disgo/handler"
	"github.com/prometheus/client_golang/prometheus"
)

type metrics struct {
	guilds       prometheus.Gauge
	interactions *prometheus.CounterVec
	duration     *prometheus.HistogramVec
	// guardianActions counts Guardian responses (alert, delete, quarantine…).
	guardianActions *prometheus.CounterVec
	// logEntries counts server-log entries by kind and result.
	logEntries *prometheus.CounterVec
	// events counts gateway events by type (Ready and Resumed show reconnects).
	events *prometheus.CounterVec
	// shardUp and shardLatency describe each shard's gateway connection.
	shardUp      *prometheus.GaugeVec
	shardLatency *prometheus.GaugeVec
	// rest counts Discord REST requests by route and status (429 = rate limited).
	rest         *prometheus.CounterVec
	restDuration *prometheus.HistogramVec
}

func newMetrics(reg prometheus.Registerer) (*metrics, error) {
	m := &metrics{
		guilds: prometheus.NewGauge(prometheus.GaugeOpts{
			Name: "niro_bot_guilds", Help: "Guilds in this process's cache.",
		}),
		interactions: prometheus.NewCounterVec(prometheus.CounterOpts{
			Name: "niro_bot_interactions_total", Help: "Interactions handled, by name and result.",
		}, []string{"name", "result"}),
		duration: prometheus.NewHistogramVec(prometheus.HistogramOpts{
			Name: "niro_bot_interaction_duration_seconds", Help: "Time to handle an interaction.",
			Buckets: []float64{.025, .05, .1, .25, .5, 1, 2.5},
		}, []string{"name"}),
		guardianActions: prometheus.NewCounterVec(prometheus.CounterOpts{
			Name: "niro_guardian_actions_total", Help: "Guardian detections, by response.",
		}, []string{"response"}),
		logEntries: prometheus.NewCounterVec(prometheus.CounterOpts{
			Name: "niro_log_entries_total", Help: "Server-log entries, by kind and result (queued, dropped, failed).",
		}, []string{"kind", "result"}),
		events: prometheus.NewCounterVec(prometheus.CounterOpts{
			Name: "niro_bot_gateway_events_total", Help: "Gateway events received, by type.",
		}, []string{"type"}),
		shardUp: prometheus.NewGaugeVec(prometheus.GaugeOpts{
			Name: "niro_bot_shard_up", Help: "1 if the shard's gateway connection is ready, else 0.",
		}, []string{"shard"}),
		shardLatency: prometheus.NewGaugeVec(prometheus.GaugeOpts{
			Name: "niro_bot_shard_latency_seconds", Help: "Gateway heartbeat latency per shard.",
		}, []string{"shard"}),
		rest: prometheus.NewCounterVec(prometheus.CounterOpts{
			Name: "niro_discord_rest_requests_total", Help: "Discord REST requests, by method, route and status code.",
		}, []string{"method", "route", "status"}),
		restDuration: prometheus.NewHistogramVec(prometheus.HistogramOpts{
			Name: "niro_discord_rest_duration_seconds", Help: "Discord REST request time, by method and route.",
			Buckets: []float64{.05, .1, .25, .5, 1, 2.5, 5, 10},
		}, []string{"method", "route"}),
	}
	for _, c := range []prometheus.Collector{m.guilds, m.interactions, m.duration, m.guardianActions, m.logEntries, m.events, m.shardUp, m.shardLatency, m.rest, m.restDuration} {
		if err := reg.Register(c); err != nil {
			return nil, err
		}
	}
	return m, nil
}

// observe is router middleware that records metrics for every interaction.
func (b *Bot) observe(next handler.Handler) handler.Handler {
	return func(e *handler.InteractionEvent) error {
		start := time.Now()
		err := next(e)
		name := interactionName(e.Interaction)
		result := "ok"
		if err != nil {
			result = "error"
		}
		b.metrics.interactions.WithLabelValues(name, result).Inc()
		b.metrics.duration.WithLabelValues(name).Observe(time.Since(start).Seconds())
		return err
	}
}

func interactionName(i discord.Interaction) string {
	switch i := i.(type) {
	case discord.ApplicationCommandInteraction:
		return "/" + i.Data.CommandName()
	case discord.ComponentInteraction:
		return "component"
	case discord.ModalSubmitInteraction:
		return "modal"
	default:
		return "other"
	}
}

// countEvent counts every gateway event by its type name.
func (m *metrics) countEvent(e bot.Event) {
	m.events.WithLabelValues(strings.TrimPrefix(fmt.Sprintf("%T", e), "*events.")).Inc()
}

// watchShards publishes each shard's status and latency until ctx ends.
func (b *Bot) watchShards(ctx context.Context, every time.Duration) {
	t := time.NewTicker(every)
	defer t.Stop()
	for {
		for s := range b.client.ShardManager.Shards() {
			id := strconv.Itoa(s.ShardID())
			up := 0.0
			if s.Status() == gateway.StatusReady {
				up = 1
			}
			b.metrics.shardUp.WithLabelValues(id).Set(up)
			b.metrics.shardLatency.WithLabelValues(id).Set(s.Latency().Seconds())
		}
		select {
		case <-ctx.Done():
			return
		case <-t.C:
		}
	}
}

// snowflakeSegment matches IDs in REST paths, so routes group into a small,
// fixed set of labels ("/channels/:id/messages/:id").
var snowflakeSegment = regexp.MustCompile(`/[0-9]{15,21}`)

// restRoute reduces a Discord API path to its route template. Interaction
// tokens and webhook tokens are dropped, so they never reach a label.
func restRoute(path string) string {
	path = strings.TrimPrefix(path, "/api/v10")
	path = snowflakeSegment.ReplaceAllString(path, "/:id")
	parts := strings.Split(path, "/")
	for i := 1; i < len(parts); i++ {
		// /interactions/:id/<token>/callback and /webhooks/:id/<token>
		if (parts[i-1] == ":id" && i >= 2 && (parts[i-2] == "interactions" || parts[i-2] == "webhooks")) && parts[i] != "" {
			parts[i] = ":token"
		}
	}
	return strings.Join(parts, "/")
}

type restTransport struct {
	next http.RoundTripper
	m    *metrics
}

// restTransport wraps the REST client's transport to record every request.
func (m *metrics) restTransport(next http.RoundTripper) http.RoundTripper {
	return restTransport{next: next, m: m}
}

func (t restTransport) RoundTrip(r *http.Request) (*http.Response, error) {
	start := time.Now()
	resp, err := t.next.RoundTrip(r)
	route := restRoute(r.URL.Path)
	status := "error"
	if err == nil {
		status = strconv.Itoa(resp.StatusCode)
	}
	t.m.rest.WithLabelValues(r.Method, route, status).Inc()
	t.m.restDuration.WithLabelValues(r.Method, route).Observe(time.Since(start).Seconds())
	return resp, err
}
