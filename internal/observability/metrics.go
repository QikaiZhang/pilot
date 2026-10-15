package observability

import "github.com/prometheus/client_golang/prometheus"

// Metrics 是 Pilot 的低基数 HTTP 运行指标集合。
// 用户输入、Trace ID 和错误文本不得进入标签。
type Metrics struct {
	HTTPRequests *prometheus.CounterVec
	HTTPDuration *prometheus.HistogramVec
	HTTPInflight prometheus.Gauge
}

func newMetrics(registry *prometheus.Registry) (*Metrics, error) {
	metrics := &Metrics{
		HTTPRequests: prometheus.NewCounterVec(
			prometheus.CounterOpts{
				Namespace: "pilot",
				Subsystem: "http",
				Name:      "requests_total",
				Help:      "Total number of HTTP requests handled by Pilot.",
			},
			[]string{"method", "route", "status_class"},
		),
		HTTPDuration: prometheus.NewHistogramVec(
			prometheus.HistogramOpts{
				Namespace: "pilot",
				Subsystem: "http",
				Name:      "request_duration_seconds",
				Help:      "HTTP request duration in seconds.",
				Buckets:   prometheus.DefBuckets,
			},
			[]string{"method", "route"},
		),
		HTTPInflight: prometheus.NewGauge(prometheus.GaugeOpts{
			Namespace: "pilot",
			Subsystem: "http",
			Name:      "inflight_requests",
			Help:      "Number of HTTP requests currently being handled by Pilot.",
		}),
	}

	collectors := []prometheus.Collector{
		metrics.HTTPRequests,
		metrics.HTTPDuration,
		metrics.HTTPInflight,
	}
	for _, collector := range collectors {
		if err := registry.Register(collector); err != nil {
			return nil, err
		}
	}
	return metrics, nil
}
