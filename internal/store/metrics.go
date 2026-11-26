package store

import (
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/prometheus/client_golang/prometheus"
)

// poolCollector exposes the Postgres connection pool's state, so an alert can
// catch an exhausted pool (requests waiting for a connection). It reads the
// pool's stats once per scrape, so the values agree with each other.
type poolCollector struct {
	pool                                           *pgxpool.Pool
	acquired, idle, max, waits, waitSecs, canceled *prometheus.Desc
}

// RegisterPoolMetrics registers the pool collector.
func RegisterPoolMetrics(reg prometheus.Registerer, pool *pgxpool.Pool) error {
	d := func(name, help string) *prometheus.Desc { return prometheus.NewDesc(name, help, nil, nil) }
	return reg.Register(&poolCollector{
		pool:     pool,
		acquired: d("niro_db_pool_connections_acquired", "Connections in use."),
		idle:     d("niro_db_pool_connections_idle", "Idle connections."),
		max:      d("niro_db_pool_connections_max", "The pool's connection limit."),
		waits:    d("niro_db_pool_acquire_waits_total", "Acquires that had to wait for a free connection."),
		waitSecs: d("niro_db_pool_acquire_wait_seconds_total", "Time spent waiting for a free connection."),
		canceled: d("niro_db_pool_acquires_canceled_total", "Acquires cancelled before a connection was free."),
	})
}

func (c *poolCollector) Describe(ch chan<- *prometheus.Desc) {
	for _, d := range []*prometheus.Desc{c.acquired, c.idle, c.max, c.waits, c.waitSecs, c.canceled} {
		ch <- d
	}
}

func (c *poolCollector) Collect(ch chan<- prometheus.Metric) {
	s := c.pool.Stat()
	ch <- prometheus.MustNewConstMetric(c.acquired, prometheus.GaugeValue, float64(s.AcquiredConns()))
	ch <- prometheus.MustNewConstMetric(c.idle, prometheus.GaugeValue, float64(s.IdleConns()))
	ch <- prometheus.MustNewConstMetric(c.max, prometheus.GaugeValue, float64(s.MaxConns()))
	ch <- prometheus.MustNewConstMetric(c.waits, prometheus.CounterValue, float64(s.EmptyAcquireCount()))
	ch <- prometheus.MustNewConstMetric(c.waitSecs, prometheus.CounterValue, s.EmptyAcquireWaitTime().Seconds())
	ch <- prometheus.MustNewConstMetric(c.canceled, prometheus.CounterValue, float64(s.CanceledAcquireCount()))
}
