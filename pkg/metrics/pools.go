package metrics

import (
	"database/sql"

	"github.com/go-redis/redis/v8"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/collectors"
)

// RegisterDatabase reads sql.DB.Stats only; it does not open connections or execute SQL.
// Use main and log as names, and register a shared main/log pool only once.
func (m *Metrics) RegisterDatabase(name string, db *sql.DB) error {
	return m.registry.Register(collectors.NewDBStatsCollector(db, name))
}

// RegisterRedis reads local pool statistics only, without issuing Redis commands.
func (m *Metrics) RegisterRedis(client *redis.Client) error {
	for _, collector := range []prometheus.Collector{
		prometheus.NewGaugeFunc(prometheus.GaugeOpts{
			Name: "new_api_redis_pool_connections", Help: "Total connections in the Redis client pool.",
		}, func() float64 { return float64(client.PoolStats().TotalConns) }),
		prometheus.NewGaugeFunc(prometheus.GaugeOpts{
			Name: "new_api_redis_pool_idle_connections", Help: "Idle connections in the Redis client pool.",
		}, func() float64 { return float64(client.PoolStats().IdleConns) }),
		prometheus.NewCounterFunc(prometheus.CounterOpts{
			Name: "new_api_redis_pool_hits_total", Help: "Redis pool acquisitions that found an available connection; not cache hits.",
		}, func() float64 { return float64(client.PoolStats().Hits) }),
		prometheus.NewCounterFunc(prometheus.CounterOpts{
			Name: "new_api_redis_pool_misses_total", Help: "Redis pool acquisitions that did not find an available connection; not cache misses.",
		}, func() float64 { return float64(client.PoolStats().Misses) }),
		prometheus.NewCounterFunc(prometheus.CounterOpts{
			Name: "new_api_redis_pool_timeouts_total", Help: "Timeouts waiting for a Redis pool connection.",
		}, func() float64 { return float64(client.PoolStats().Timeouts) }),
		prometheus.NewCounterFunc(prometheus.CounterOpts{
			Name: "new_api_redis_pool_stale_connections_total", Help: "Stale Redis pool connections removed.",
		}, func() float64 { return float64(client.PoolStats().StaleConns) }),
	} {
		if err := m.registry.Register(collector); err != nil {
			return err
		}
	}
	return nil
}
