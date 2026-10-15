package observability

import (
	"context"
	"errors"
	"testing"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"
)

func TestStartSpanPropagatesContextAndRecordsError(t *testing.T) {
	exporter := tracetest.NewInMemoryExporter()
	tracerProvider := trace.NewTracerProvider(trace.WithSyncer(exporter))
	otel.SetTracerProvider(tracerProvider)

	ctx, parent := StartSpan(context.Background(), "parent")
	if len(ContextAttrs(ctx)) != 2 {
		t.Fatalf("ContextAttrs() length = %d, want 2", len(ContextAttrs(ctx)))
	}
	_, child := StartSpan(ctx, "child")
	child.End(nil)
	parent.End(errors.New("expected test failure"))

	spans := exporter.GetSpans()
	if len(spans) != 2 {
		t.Fatalf("span count = %d, want 2", len(spans))
	}
	var parentSpan, childSpan tracetest.SpanStub
	for _, span := range spans {
		switch span.Name {
		case "parent":
			parentSpan = span
		case "child":
			childSpan = span
		}
	}
	if childSpan.Parent.SpanID() != parentSpan.SpanContext.SpanID() {
		t.Fatalf("child parent = %s, want %s", childSpan.Parent.SpanID(), parentSpan.SpanContext.SpanID())
	}
	if parentSpan.Status.Code != codes.Error {
		t.Fatal("parent span status is not error")
	}
}
