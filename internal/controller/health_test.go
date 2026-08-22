package controller

import (
	"context"
	"encoding/json"
	"errors"
	"net/http/httptest"
	"testing"

	"Pilot/internal/deps"
)

func TestLiveReturnsOK(t *testing.T) {
	h := NewHealth(nil)
	rec := httptest.NewRecorder()
	h.Live(rec, httptest.NewRequest("GET", "/api/v1/health/live", nil))

	if rec.Code != 200 {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	var body map[string]string
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if body["status"] != "ok" {
		t.Fatalf("status body = %q, want ok", body["status"])
	}
}

func TestReadyAllOK(t *testing.T) {
	h := NewHealth([]deps.Dependency{
		{Name: "redis", Check: func(context.Context) error { return nil }},
		{Name: "mysql", Check: func(context.Context) error { return nil }},
	})
	rec := httptest.NewRecorder()
	h.Ready(rec, httptest.NewRequest("GET", "/api/v1/health/ready", nil))

	if rec.Code != 200 {
		t.Fatalf("status = %d, want 200; body=%s", rec.Code, rec.Body.String())
	}
	var body readyResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if body.Status != "ok" {
		t.Fatalf("status = %q, want ok", body.Status)
	}
	if len(body.Checks) != 2 {
		t.Fatalf("checks len = %d, want 2", len(body.Checks))
	}
}

func TestReadyReportsFailurePerItem(t *testing.T) {
	h := NewHealth([]deps.Dependency{
		{Name: "redis", Check: func(context.Context) error { return nil }},
		{Name: "mysql", Check: func(context.Context) error { return errors.New("mysql: connection refused") }},
	})
	rec := httptest.NewRecorder()
	h.Ready(rec, httptest.NewRequest("GET", "/api/v1/health/ready", nil))

	if rec.Code != 503 {
		t.Fatalf("status = %d, want 503; body=%s", rec.Code, rec.Body.String())
	}
	var body readyResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if body.Status != "unavailable" {
		t.Fatalf("status = %q, want unavailable", body.Status)
	}
	if len(body.Checks) != 2 {
		t.Fatalf("checks len = %d, want 2", len(body.Checks))
	}
	if body.Checks[0].Status != "ok" || body.Checks[0].Error != "" {
		t.Fatalf("redis check = %+v, want ok", body.Checks[0])
	}
	if body.Checks[1].Status != "error" || body.Checks[1].Error == "" {
		t.Fatalf("mysql check = %+v, want error with message", body.Checks[1])
	}
}

func TestWriteErrorEnvelope(t *testing.T) {
	rec := httptest.NewRecorder()
	WriteError(rec, 400, "bad_request", "something went wrong")

	if rec.Code != 400 {
		t.Fatalf("status = %d, want 400", rec.Code)
	}
	var body errorBody
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if body.Code != "bad_request" || body.Message != "something went wrong" {
		t.Fatalf("body = %+v", body)
	}
}
