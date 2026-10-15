package observability

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/prometheus/client_golang/prometheus"
	dto "github.com/prometheus/client_model/go"
	"go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"
)

func newTestProvider(t *testing.T) (*Provider, *tracetest.InMemoryExporter) {
	t.Helper()
	registry := prometheus.NewRegistry()
	metrics, err := newMetrics(registry)
	if err != nil {
		t.Fatalf("newMetrics() error = %v", err)
	}
	exporter := tracetest.NewInMemoryExporter()
	tracerProvider := trace.NewTracerProvider(trace.WithSyncer(exporter))
	return &Provider{
		registry:       registry,
		metrics:        metrics,
		tracerProvider: tracerProvider,
		shutdown:       tracerProvider.Shutdown,
	}, exporter
}

func TestMiddlewareRecordsRequestMetricsAndTrace(t *testing.T) {
	provider, exporter := newTestProvider(t)
	handler := http.NewServeMux()
	handler.Handle("GET /hello", http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusAccepted)
	}))
	server := httptest.NewServer(provider.Middleware(handler))
	defer server.Close()

	response, err := http.Get(server.URL + "/hello")
	if err != nil {
		t.Fatalf("GET /hello error = %v", err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusAccepted {
		t.Fatalf("status = %d, want %d", response.StatusCode, http.StatusAccepted)
	}
	if response.Header.Get("X-Trace-ID") == "" {
		t.Fatal("X-Trace-ID header is empty")
	}
	spans := exporter.GetSpans()
	if len(spans) != 1 || spans[0].Name != "http.request" {
		t.Fatalf("spans = %+v, want one http.request span", spans)
	}

	metricFamilies, err := provider.Registry().Gather()
	if err != nil {
		t.Fatalf("Gather() error = %v", err)
	}
	if !hasMetricFamily(metricFamilies, "pilot_http_requests_total") {
		t.Fatal("pilot_http_requests_total was not gathered")
	}
	if !hasMetricFamily(metricFamilies, "pilot_http_request_duration_seconds") {
		t.Fatal("pilot_http_request_duration_seconds was not gathered")
	}
}

func hasMetricFamily(families []*dto.MetricFamily, name string) bool {
	for _, family := range families {
		if family.GetName() == name {
			return true
		}
	}
	return false
}
