package observability

import (
	"context"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracegrpc"
	"go.opentelemetry.io/otel/sdk/resource"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/trace"
)

// Config 控制应用侧观测 provider。观测 exporter 故障不应阻断业务启动。
type Config struct {
	Enabled            bool
	ServiceName        string
	Environment        string
	OTLPExportEndpoint string
	OTLPInsecure       bool
	ExportTimeout      time.Duration
	SampleRatio        float64
}

// Provider 聚合 Prometheus registry 和 OpenTelemetry tracer 生命周期。
type Provider struct {
	registry       *prometheus.Registry
	metrics        *Metrics
	tracerProvider trace.TracerProvider
	shutdown       func(context.Context) error
	shutdownOnce   sync.Once
	shutdownErr    error
}

// New 创建观测 provider。禁用或未配置 OTLP endpoint 时使用 no-op tracer，
// 但仍保留本地 metrics registry，便于运行态诊断。
func New(ctx context.Context, cfg Config) (*Provider, error) {
	registry := prometheus.NewRegistry()
	metrics, err := newMetrics(registry)
	if err != nil {
		return nil, err
	}

	serviceName := cfg.ServiceName
	if serviceName == "" {
		serviceName = "pilot"
	}
	provider := &Provider{
		registry: registry,
		metrics:  metrics,
	}
	noop := trace.NewNoopTracerProvider()
	provider.tracerProvider = noop
	provider.shutdown = func(context.Context) error { return nil }

	if !cfg.Enabled || cfg.OTLPExportEndpoint == "" {
		return provider, nil
	}

	endpoint := cfg.OTLPExportEndpoint
	if !strings.Contains(endpoint, "://") {
		scheme := "https"
		if cfg.OTLPInsecure {
			scheme = "http"
		}
		endpoint = scheme + "://" + endpoint
	}
	exporterOptions := []otlptracegrpc.Option{
		otlptracegrpc.WithEndpointURL(endpoint),
	}
	if cfg.OTLPInsecure {
		exporterOptions = append(exporterOptions, otlptracegrpc.WithInsecure())
	}
	exporter, err := otlptracegrpc.New(ctx, exporterOptions...)
	if err != nil {
		return nil, err
	}

	resourceAttrs := []attribute.KeyValue{
		attribute.String("service.name", serviceName),
		attribute.String("deployment.environment", cfg.Environment),
	}
	tracerOptions := []sdktrace.TracerProviderOption{
		sdktrace.WithResource(resource.NewWithAttributes("", resourceAttrs...)),
	}
	tracerOptions = append(tracerOptions, sdktrace.WithSampler(sdktrace.ParentBased(
		sdktrace.TraceIDRatioBased(normalizeSampleRatio(cfg.SampleRatio)),
	)))
	batchOptions := make([]sdktrace.BatchSpanProcessorOption, 0, 1)
	if cfg.ExportTimeout > 0 {
		batchOptions = append(batchOptions, sdktrace.WithExportTimeout(cfg.ExportTimeout))
	}
	tracerOptions = append(tracerOptions, sdktrace.WithBatcher(exporter, batchOptions...))
	tracerProvider := sdktrace.NewTracerProvider(tracerOptions...)
	provider.tracerProvider = tracerProvider
	provider.shutdown = tracerProvider.Shutdown
	otel.SetTracerProvider(tracerProvider)
	return provider, nil
}

func normalizeSampleRatio(ratio float64) float64 {
	if ratio < 0 {
		return 0
	}
	if ratio > 1 {
		return 1
	}
	return ratio
}

// Registry 返回本 provider 的 Prometheus registry，避免污染默认全局 registry。
func (p *Provider) Registry() *prometheus.Registry {
	if p == nil {
		return prometheus.NewRegistry()
	}
	return p.registry
}

// Metrics 返回 HTTP 指标集合，业务包后续可在此 registry 上注册自己的指标。
func (p *Provider) Metrics() *Metrics {
	if p == nil {
		return nil
	}
	return p.metrics
}

// Tracer 返回带 service provider 的 tracer。
func (p *Provider) Tracer(name string) trace.Tracer {
	if p == nil || p.tracerProvider == nil {
		return trace.NewNoopTracerProvider().Tracer(name)
	}
	return p.tracerProvider.Tracer(name)
}

// MetricsHandler 暴露 Prometheus text exposition endpoint。
func (p *Provider) MetricsHandler() http.Handler {
	return promhttp.HandlerFor(p.Registry(), promhttp.HandlerOpts{})
}

// Shutdown 在服务退出时 flush exporter；调用幂等。
func (p *Provider) Shutdown(ctx context.Context) error {
	if p == nil {
		return nil
	}
	p.shutdownOnce.Do(func() {
		if p.shutdown != nil {
			p.shutdownErr = p.shutdown(ctx)
		}
	})
	return p.shutdownErr
}
