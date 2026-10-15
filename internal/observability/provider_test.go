package observability

import (
	"context"
	"testing"
	"time"
)

func TestNormalizeSampleRatio(t *testing.T) {
	for _, test := range []struct {
		name string
		in   float64
		want float64
	}{
		{name: "negative", in: -1, want: 0},
		{name: "within range", in: 0.25, want: 0.25},
		{name: "greater than one", in: 2, want: 1},
	} {
		t.Run(test.name, func(t *testing.T) {
			if got := normalizeSampleRatio(test.in); got != test.want {
				t.Fatalf("normalizeSampleRatio(%v) = %v, want %v", test.in, got, test.want)
			}
		})
	}
}

func TestNewDisabledProviderKeepsMetricsAvailable(t *testing.T) {
	provider, err := New(context.Background(), Config{Enabled: false})
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	provider.Metrics().HTTPRequests.WithLabelValues("GET", "/metrics", "2xx").Inc()
	provider.Metrics().HTTPDuration.WithLabelValues("GET", "/metrics").Observe(0.01)
	metricFamilies, err := provider.Registry().Gather()
	if err != nil {
		t.Fatalf("Gather() error = %v", err)
	}
	if len(metricFamilies) != 3 {
		t.Fatalf("metric family count = %d, want 3", len(metricFamilies))
	}

	shutdownCtx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	if err := provider.Shutdown(shutdownCtx); err != nil {
		t.Fatalf("Shutdown() error = %v", err)
	}
	if err := provider.Shutdown(shutdownCtx); err != nil {
		t.Fatalf("second Shutdown() error = %v", err)
	}
}
