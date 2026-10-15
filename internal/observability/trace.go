package observability

import (
	"context"
	"log/slog"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/trace"
)

const applicationTracerName = "Pilot/application"

// Span 是业务包使用的最小 Trace 门面，避免把 exporter 和 provider 细节扩散到业务层。
type Span struct {
	span trace.Span
}

// StartSpan 从请求 context 派生一个业务 Span。
func StartSpan(ctx context.Context, name string) (context.Context, *Span) {
	if ctx == nil {
		ctx = context.Background()
	}
	ctx, span := otel.Tracer(applicationTracerName).Start(ctx, name)
	return ctx, &Span{span: span}
}

// End 结束业务 Span，并把业务错误转换为 Trace Error status。
func (s *Span) End(err error) {
	if s == nil || s.span == nil {
		return
	}
	if err != nil {
		s.span.RecordError(err)
		s.span.SetStatus(codes.Error, err.Error())
	}
	s.span.End()
	s.span = nil
}

// ContextAttrs 返回可安全附加到结构化日志的 trace/span ID。
func ContextAttrs(ctx context.Context) []slog.Attr {
	if ctx == nil {
		return nil
	}
	spanContext := trace.SpanContextFromContext(ctx)
	if !spanContext.IsValid() {
		return nil
	}
	return []slog.Attr{
		slog.String("trace_id", spanContext.TraceID().String()),
		slog.String("span_id", spanContext.SpanID().String()),
	}
}

// LogAttrs 统一把 trace/span ID 加入结构化日志。
func LogAttrs(ctx context.Context, level slog.Level, message string, attrs ...slog.Attr) {
	logAttrs := append(ContextAttrs(ctx), attrs...)
	slog.Default().LogAttrs(ctx, level, message, logAttrs...)
}
