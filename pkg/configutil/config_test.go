package configutil

import (
	"testing"
	"time"
)

var allKeys = []string{
	"APP_ADDR", "APP_ENV", "ALLOW_DEGRADED", "API_KEY",
	"REDIS_ADDR", "REDIS_PASSWORD", "REDIS_DB", "REDIS_MEMORY_TTL", "REDIS_MAX_MESSAGES",
	"MYSQL_DSN",
	"ES_URL", "ES_USERNAME", "ES_PASSWORD", "ES_KNOWLEDGE_INDEX", "ES_LOG_INDEX",
	"LLM_MODE", "LLM_BASE_URL", "LLM_API_KEY", "LLM_MODEL",
	"EMBEDDING_MODE", "EMBEDDING_DIM",
	"RATE_LIMIT_REQUESTS", "RATE_LIMIT_WINDOW",
	"OTEL_EXPORTER_OTLP_ENDPOINT",
}

func clearAll(t *testing.T) {
	t.Helper()
	for _, k := range allKeys {
		t.Setenv(k, "")
	}
}

func TestLoadDefaults(t *testing.T) {
	clearAll(t)
	cfg := Load()

	if cfg.App.Addr != ":8080" {
		t.Errorf("App.Addr = %q, want %q", cfg.App.Addr, ":8080")
	}
	if cfg.App.Env != "local" {
		t.Errorf("App.Env = %q, want %q", cfg.App.Env, "local")
	}
	if !cfg.App.AllowDegraded {
		t.Error("App.AllowDegraded = false, want true")
	}
	if cfg.Redis.Addr != "127.0.0.1:6379" {
		t.Errorf("Redis.Addr = %q, want 127.0.0.1:6379", cfg.Redis.Addr)
	}
	if cfg.Redis.MemoryTTL != 24*time.Hour {
		t.Errorf("Redis.MemoryTTL = %v, want 24h", cfg.Redis.MemoryTTL)
	}
	if cfg.Redis.MaxMessages != 20 {
		t.Errorf("Redis.MaxMessages = %d, want 20", cfg.Redis.MaxMessages)
	}
	if cfg.MySQL.DSN == "" {
		t.Error("MySQL.DSN is empty")
	}
	if cfg.ES.URL != "http://127.0.0.1:9200" {
		t.Errorf("ES.URL = %q, want http://127.0.0.1:9200", cfg.ES.URL)
	}
	if cfg.LLM.Mode != "mock" {
		t.Errorf("LLM.Mode = %q, want mock", cfg.LLM.Mode)
	}
	if cfg.Embedding.Dim != 32 {
		t.Errorf("Embedding.Dim = %d, want 32", cfg.Embedding.Dim)
	}
	if cfg.RateLimit.Requests != 60 || cfg.RateLimit.Window != time.Minute {
		t.Errorf("RateLimit = %+v, want 60/1m", cfg.RateLimit)
	}
	if cfg.OTEL.OTLPExportEndpoint != "127.0.0.1:4317" {
		t.Errorf("OTEL endpoint = %q, want 127.0.0.1:4317", cfg.OTEL.OTLPExportEndpoint)
	}
}

func TestLoadFromEnv(t *testing.T) {
	clearAll(t)
	t.Setenv("APP_ADDR", ":9999")
	t.Setenv("REDIS_DB", "3")
	t.Setenv("REDIS_MEMORY_TTL", "1h")
	t.Setenv("ALLOW_DEGRADED", "false")
	t.Setenv("EMBEDDING_DIM", "64")

	cfg := Load()
	if cfg.App.Addr != ":9999" {
		t.Errorf("App.Addr = %q, want :9999", cfg.App.Addr)
	}
	if cfg.Redis.DB != 3 {
		t.Errorf("Redis.DB = %d, want 3", cfg.Redis.DB)
	}
	if cfg.Redis.MemoryTTL != time.Hour {
		t.Errorf("Redis.MemoryTTL = %v, want 1h", cfg.Redis.MemoryTTL)
	}
	if cfg.App.AllowDegraded {
		t.Error("App.AllowDegraded = true, want false")
	}
	if cfg.Embedding.Dim != 64 {
		t.Errorf("Embedding.Dim = %d, want 64", cfg.Embedding.Dim)
	}
}
