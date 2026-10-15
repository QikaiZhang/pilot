package observability

import (
	"context"
	"fmt"
	"net/http"
	"strings"
	"time"

	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/trace"
)

const tracerName = "Pilot/internal/observability/http"

// Middleware 为每个 HTTP 请求创建 root span，并记录低基数请求指标。
func (p *Provider) Middleware(next http.Handler) http.Handler {
	if p == nil || next == nil {
		return next
	}

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		route := requestRoute(r)
		ctx := propagation.TraceContext{}.Extract(r.Context(), propagation.HeaderCarrier(r.Header))
		ctx, span := p.Tracer(tracerName).Start(ctx, "http.request", trace.WithSpanKind(trace.SpanKindServer))
		traceID := span.SpanContext().TraceID().String()
		if span.SpanContext().IsValid() {
			w.Header().Set("X-Trace-ID", traceID)
		}

		method := strings.ToUpper(r.Method)
		start := time.Now()
		p.metrics.HTTPInflight.Inc()
		writer := &responseWriter{ResponseWriter: w}
		defer func() {
			p.metrics.HTTPInflight.Dec()
			status := writer.status
			if status == 0 {
				status = http.StatusOK
			}
			statusClass := fmt.Sprintf("%dxx", status/100)
			p.metrics.HTTPRequests.WithLabelValues(method, route, statusClass).Inc()
			p.metrics.HTTPDuration.WithLabelValues(method, route).Observe(time.Since(start).Seconds())
			span.SetAttributes(
				attribute.String("http.route", route),
				attribute.Int("http.status_code", status),
			)
			if status >= http.StatusInternalServerError {
				span.SetStatus(codes.Error, http.StatusText(status))
			}
			span.End()
		}()

		next.ServeHTTP(writer, r.WithContext(ctx))
	})
}

func requestRoute(r *http.Request) string {
	if r == nil {
		return "unknown"
	}
	if route := strings.TrimSpace(r.Pattern); route != "" {
		return route
	}
	return "unknown"
}

type responseWriter struct {
	http.ResponseWriter
	status      int
	wroteHeader bool
}

func (w *responseWriter) WriteHeader(status int) {
	if w.wroteHeader {
		return
	}
	w.status = status
	w.wroteHeader = true
	w.ResponseWriter.WriteHeader(status)
}

func (w *responseWriter) Write(body []byte) (int, error) {
	if !w.wroteHeader {
		w.WriteHeader(http.StatusOK)
	}
	return w.ResponseWriter.Write(body)
}

// Flush 保留 SSE handler 需要的 http.Flusher 能力。
func (w *responseWriter) Flush() {
	if !w.wroteHeader {
		w.WriteHeader(http.StatusOK)
	}
	if flusher, ok := w.ResponseWriter.(http.Flusher); ok {
		flusher.Flush()
	}
}

// Unwrap 让 ResponseController 和其他标准库适配器访问底层 writer。
func (w *responseWriter) Unwrap() http.ResponseWriter {
	return w.ResponseWriter
}

// contextWithTrace 保留给日志和业务 adapter 使用，避免调用方重复解析 Span。
func contextWithTrace(ctx context.Context) (trace.TraceID, trace.SpanID, bool) {
	spanContext := trace.SpanContextFromContext(ctx)
	if !spanContext.IsValid() {
		return trace.TraceID{}, trace.SpanID{}, false
	}
	return spanContext.TraceID(), spanContext.SpanID(), true
}
