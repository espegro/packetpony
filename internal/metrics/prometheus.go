package metrics

import (
	"fmt"
	"net/http"

	"github.com/espegro/packetpony/internal/config"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

// ProxyMetrics holds all Prometheus metrics for the proxy
type ProxyMetrics struct {
	ConnectionsTotal     *prometheus.CounterVec
	ConnectionsActive    *prometheus.GaugeVec
	BytesTransferred     *prometheus.CounterVec
	PacketsTransferred   *prometheus.CounterVec
	ConnectionDuration   *prometheus.HistogramVec
	RateLimitDrops       *prometheus.CounterVec
	ACLDrops             *prometheus.CounterVec
	Errors               *prometheus.CounterVec
}

// NewProxyMetrics creates and registers Prometheus metrics
func NewProxyMetrics() *ProxyMetrics {
	metrics := &ProxyMetrics{
		ConnectionsTotal: prometheus.NewCounterVec(
			prometheus.CounterOpts{
				Name: "packetpony_connections_total",
				Help: "Total number of connections",
			},
			[]string{"listener", "protocol", "status"},
		),
		ConnectionsActive: prometheus.NewGaugeVec(
			prometheus.GaugeOpts{
				Name: "packetpony_connections_active",
				Help: "Number of active connections",
			},
			[]string{"listener", "protocol"},
		),
		BytesTransferred: prometheus.NewCounterVec(
			prometheus.CounterOpts{
				Name: "packetpony_bytes_transferred_total",
				Help: "Total bytes transferred",
			},
			[]string{"listener", "direction"},
		),
		PacketsTransferred: prometheus.NewCounterVec(
			prometheus.CounterOpts{
				Name: "packetpony_packets_transferred_total",
				Help: "Total packets transferred (UDP only)",
			},
			[]string{"listener", "direction"},
		),
		ConnectionDuration: prometheus.NewHistogramVec(
			prometheus.HistogramOpts{
				Name:    "packetpony_connection_duration_seconds",
				Help:    "Connection duration in seconds",
				Buckets: prometheus.ExponentialBuckets(0.001, 2, 15), // 1ms to ~16s
			},
			[]string{"listener", "protocol"},
		),
		RateLimitDrops: prometheus.NewCounterVec(
			prometheus.CounterOpts{
				Name: "packetpony_rate_limit_drops_total",
				Help: "Total connections dropped due to rate limiting",
			},
			[]string{"listener", "reason"},
		),
		ACLDrops: prometheus.NewCounterVec(
			prometheus.CounterOpts{
				Name: "packetpony_acl_drops_total",
				Help: "Total connections dropped due to ACL",
			},
			[]string{"listener"},
		),
		Errors: prometheus.NewCounterVec(
			prometheus.CounterOpts{
				Name: "packetpony_errors_total",
				Help: "Total errors encountered",
			},
			[]string{"listener", "type"},
		),
	}

	// Register all metrics
	prometheus.MustRegister(metrics.ConnectionsTotal)
	prometheus.MustRegister(metrics.ConnectionsActive)
	prometheus.MustRegister(metrics.BytesTransferred)
	prometheus.MustRegister(metrics.PacketsTransferred)
	prometheus.MustRegister(metrics.ConnectionDuration)
	prometheus.MustRegister(metrics.RateLimitDrops)
	prometheus.MustRegister(metrics.ACLDrops)
	prometheus.MustRegister(metrics.Errors)

	return metrics
}

// StartMetricsServer starts the HTTP server for Prometheus metrics
func StartMetricsServer(cfg config.PrometheusConfig) error {
	if !cfg.Enabled {
		return nil
	}

	http.Handle(cfg.Path, promhttp.Handler())

	go func() {
		if err := http.ListenAndServe(cfg.ListenAddress, nil); err != nil {
			fmt.Printf("Failed to start metrics server: %v\n", err)
		}
	}()

	return nil
}
