package tools

import (
	"context"
	"errors"
	"testing"
)

func TestHealthCheckExecuteReportsEveryDependency(t *testing.T) {
	tool := NewHealthCheck([]HealthDependency{
		{Name: "redis", Check: func(context.Context) error { return nil }},
		{Name: "mysql", Check: func(context.Context) error { return errors.New("connection refused") }},
	})

	out, err := tool.Execute(context.Background(), nil)
	if err != nil {
		t.Fatal(err)
	}
	result, ok := out.(HealthCheckResult)
	if !ok {
		t.Fatalf("output type = %T, want HealthCheckResult", out)
	}
	if result.Healthy {
		t.Fatal("healthy = true, want false")
	}
	if len(result.Services) != 2 {
		t.Fatalf("services = %d, want 2", len(result.Services))
	}
	if result.Services[0].Status != "up" || result.Services[1].Status != "down" {
		t.Fatalf("services = %+v, want redis up and mysql down", result.Services)
	}
}

func TestHealthCheckInvalidInput(t *testing.T) {
	tool := NewHealthCheck(nil)
	if _, err := tool.Execute(context.Background(), []byte(`[]`)); err == nil {
		t.Fatal("expected object input validation error")
	}
}

func TestHealthCheckMissingProbeIsDown(t *testing.T) {
	tool := NewHealthCheck([]HealthDependency{{Name: "redis"}})
	out, err := tool.Execute(context.Background(), nil)
	if err != nil {
		t.Fatal(err)
	}
	result := out.(HealthCheckResult)
	if result.Healthy || result.Services[0].Status != "down" {
		t.Fatalf("result = %+v, want unhealthy dependency", result)
	}
}
