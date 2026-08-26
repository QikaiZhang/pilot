// Package configutil 把环境变量映射为 Pilot 的配置结构。
package configutil

import (
	"os"
	"strconv"
	"time"
)

// Config 是 Pilot 服务全部配置的聚合。
type Config struct {
	App       AppConfig
	Redis     RedisConfig
	MySQL     MySQLConfig
	ES        ESConfig
	LLM       LLMConfig
	Embedding EmbeddingConfig
	RateLimit RateLimitConfig
	OTEL      OTELConfig
}

// AppConfig 是 HTTP 服务与应用级配置。
type AppConfig struct {
	Addr          string // HTTP 监听地址，如 :8080
	Env           string
	AllowDegraded bool
	APIKey        string
}

// RedisConfig 是短期记忆相关配置。
type RedisConfig struct {
	Addr        string
	Password    string
	DB          int
	MemoryTTL   time.Duration
	MaxMessages int
}

// MySQLConfig 是长期历史相关配置。
type MySQLConfig struct {
	DSN string
}

// ESConfig 是知识库与日志索引相关配置。
type ESConfig struct {
	URL            string
	Username       string
	Password       string
	KnowledgeIndex string
	LogIndex       string
}

// LLMConfig 是模型供应商相关配置。
// Temperature/MaxTokens/Timeout 在 LLM_MODE=mock 时不会被消费，
// 仅在选用 eino/openai 时生效，作为服务端给模型的默认参数。
type LLMConfig struct {
	Mode        string
	BaseURL     string
	APIKey      string
	Model       string
	Temperature float32
	MaxTokens   int
	Timeout     time.Duration
}

// EmbeddingConfig 是向量化相关配置。
type EmbeddingConfig struct {
	Mode string
	Dim  int
}

// RateLimitConfig 是接口限流相关配置。
type RateLimitConfig struct {
	Requests int
	Window   time.Duration
}

// OTELConfig 是可观测性上报相关配置。
type OTELConfig struct {
	OTLPExportEndpoint string
}

// Load 从进程环境变量构建 Config。解析失败时回退到默认值，
// 便于本地开发时只提供部分变量。
func Load() Config {
	return Config{
		App: AppConfig{
			Addr:          getString("APP_ADDR", ":8080"),
			Env:           getString("APP_ENV", "local"),
			AllowDegraded: getBool("ALLOW_DEGRADED", true),
			APIKey:        getString("API_KEY", ""),
		},
		Redis: RedisConfig{
			Addr:        getString("REDIS_ADDR", "127.0.0.1:6379"),
			Password:    getString("REDIS_PASSWORD", ""),
			DB:          getInt("REDIS_DB", 0),
			MemoryTTL:   getDuration("REDIS_MEMORY_TTL", 24*time.Hour),
			MaxMessages: getInt("REDIS_MAX_MESSAGES", 20),
		},
		MySQL: MySQLConfig{
			DSN: getString("MYSQL_DSN", "pilot:pilot@tcp(127.0.0.1:3306)/pilot?parseTime=true&charset=utf8mb4"),
		},
		ES: ESConfig{
			URL:            getString("ES_URL", "http://127.0.0.1:9200"),
			Username:       getString("ES_USERNAME", ""),
			Password:       getString("ES_PASSWORD", ""),
			KnowledgeIndex: getString("ES_KNOWLEDGE_INDEX", "pilot_knowledge"),
			LogIndex:       getString("ES_LOG_INDEX", "pilot_logs"),
		},
		LLM: LLMConfig{
			Mode:        getString("LLM_MODE", "mock"),
			BaseURL:     getString("LLM_BASE_URL", ""),
			APIKey:      getString("LLM_API_KEY", ""),
			Model:       getString("LLM_MODEL", ""),
			Temperature: getFloat("LLM_TEMPERATURE", 0),
			MaxTokens:   getInt("LLM_MAX_TOKENS", 0),
			Timeout:     getDuration("LLM_TIMEOUT", 0),
		},
		Embedding: EmbeddingConfig{
			Mode: getString("EMBEDDING_MODE", "mock"),
			Dim:  getInt("EMBEDDING_DIM", 32),
		},
		RateLimit: RateLimitConfig{
			Requests: getInt("RATE_LIMIT_REQUESTS", 60),
			Window:   getDuration("RATE_LIMIT_WINDOW", time.Minute),
		},
		OTEL: OTELConfig{
			OTLPExportEndpoint: getString("OTEL_EXPORTER_OTLP_ENDPOINT", "127.0.0.1:4317"),
		},
	}
}

func getString(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

func getInt(key string, fallback int) int {
	v := os.Getenv(key)
	if v == "" {
		return fallback
	}
	n, err := strconv.Atoi(v)
	if err != nil {
		return fallback
	}
	return n
}

func getFloat(key string, fallback float32) float32 {
	v := os.Getenv(key)
	if v == "" {
		return fallback
	}
	f, err := strconv.ParseFloat(v, 32)
	if err != nil {
		return fallback
	}
	return float32(f)
}

func getBool(key string, fallback bool) bool {
	v := os.Getenv(key)
	if v == "" {
		return fallback
	}
	b, err := strconv.ParseBool(v)
	if err != nil {
		return fallback
	}
	return b
}

func getDuration(key string, fallback time.Duration) time.Duration {
	v := os.Getenv(key)
	if v == "" {
		return fallback
	}
	d, err := time.ParseDuration(v)
	if err != nil {
		return fallback
	}
	return d
}
