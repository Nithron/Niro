package discord

import (
	"time"

	"github.com/disgoorg/disgo/discord"
	"github.com/disgoorg/disgo/handler"
	"github.com/prometheus/client_golang/prometheus"
)

type metrics struct {
	guilds       prometheus.Gauge
	interactions *prometheus.CounterVec
	duration     *prometheus.HistogramVec
}

func newMetrics(reg prometheus.Registerer) (*metrics, error) {
	m := &metrics{
		guilds: prometheus.NewGauge(prometheus.GaugeOpts{
			Name: "nithro_bot_guilds", Help: "Guilds in this process's cache.",
		}),
		interactions: prometheus.NewCounterVec(prometheus.CounterOpts{
			Name: "nithro_bot_interactions_total", Help: "Interactions handled, by name and result.",
		}, []string{"name", "result"}),
		duration: prometheus.NewHistogramVec(prometheus.HistogramOpts{
			Name: "nithro_bot_interaction_duration_seconds", Help: "Time to handle an interaction.",
			Buckets: []float64{.025, .05, .1, .25, .5, 1, 2.5},
		}, []string{"name"}),
	}
	for _, c := range []prometheus.Collector{m.guilds, m.interactions, m.duration} {
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
