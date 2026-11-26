package store

import (
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/prometheus/client_golang/prometheus"
)

// RegisterPoolMetrics exposes the Postgres connection pool's state, so an
// alert can catch an exhausted pool (requests waiting for a connection).
func RegisterPoolMetrics(reg prometheus.Registerer, pool *pgxpool.Pool) error {
	gauge := func(name, help string, f func(*pgxpool.Stat) float64) prometheus.Collector {
		return prometheus.NewGaugeFunc(prometheus.GaugeOpts{Name: name, Help: help}, func() float64 { return f(pool.Stat()) })
	}
	counter := func(name, help string, f func(*pgxpool.Stat) float64) prometheus.Collector {
		return prometheus.NewCounterFunc(prometheus.CounterOpts{Name: name, Help: help}, func() float64 { return f(pool.Stat()) })
	}
	for _, c := range []prometheus.Collector{
		gauge("niro_db_pool_connections_acquired", "Connections in use.", func(s *pgxpool.Stat) float64 { return float64(s.AcquiredConns()) }),
		gauge("niro_db_pool_connections_idle", "Idle connections.", func(s *pgxpool.Stat) float64 { return float64(s.IdleConns()) }),
		gauge("niro_db_pool_connections_max", "The pool's connection limit.", func(s *pgxpool.Stat) float64 { return float64(s.MaxConns()) }),
		counter("niro_db_pool_acquire_waits_total", "Acquires that had to wait for a free connection.", func(s *pgxpool.Stat) float64 { return float64(s.EmptyAcquireCount()) }),
		counter("niro_db_pool_acquire_wait_seconds_total", "Time spent waiting for a free connection.", func(s *pgxpool.Stat) float64 { return s.EmptyAcquireWaitTime().Seconds() }),
		counter("niro_db_pool_acquires_canceled_total", "Acquires cancelled before a connection was free.", func(s *pgxpool.Stat) float64 { return float64(s.CanceledAcquireCount()) }),
	} {
		if err := reg.Register(c); err != nil {
			return err
		}
	}
	return nil
}
