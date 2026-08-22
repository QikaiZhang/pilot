// Package deps 提供启动期依赖连通性检查（Redis、MySQL、Elasticsearch）。
// 检查失败的错误只包含依赖名和经过净化的信息，不包含密码或完整 DSN。
package deps

import (
	"context"
	"database/sql"
	"fmt"
	"net/http"
	"net/url"
	"time"

	_ "github.com/go-sql-driver/mysql"
	"github.com/redis/go-redis/v9"
)

// silentLogger 抑制 go-redis 默认日志，避免依赖不可用时刷屏。
type silentLogger struct{}

func (silentLogger) Printf(context.Context, string, ...any) {}

func init() {
	redis.SetLogger(silentLogger{})
}

// Dependency 是一次可执行的连通性检查。
type Dependency struct {
	Name  string
	Check func(ctx context.Context) error
}

// Result 是一次依赖检查的结果。
type Result struct {
	Name      string        `json:"name"`
	OK        bool          `json:"ok"`
	Latency   time.Duration `json:"-"`
	Err       error         `json:"-"`
	LatencyMS int64         `json:"latency_ms"`
}

// CheckAll 逐个检查依赖并聚合结果，单个依赖失败不跳过其余检查。
// WARNING S级核心逻辑：理解后必须删除本区域，并从零独立重写，禁止直接复用。
// 删除范围：从 BEGIN S_LEVEL_REFERENCE 到 END S_LEVEL_REFERENCE。
// BEGIN S_LEVEL_REFERENCE
func CheckAll(ctx context.Context, deps []Dependency) []Result {
	results := make([]Result, 0, len(deps))
	for _, dep := range deps {
		start := time.Now()
		err := dep.Check(ctx)
		latency := time.Since(start)
		results = append(results, Result{
			Name:      dep.Name,
			OK:        err == nil,
			Latency:   latency,
			LatencyMS: latency.Milliseconds(),
			Err:       err,
		})
	}
	return results
}

// END S_LEVEL_REFERENCE

// AllOK 判断一组检查结果是否全部通过。
func AllOK(results []Result) bool {
	for _, r := range results {
		if !r.OK {
			return false
		}
	}
	return true
}

// Redis 构造 Redis 连通性检查，使用 PING 命令。
func Redis(addr, password string, db int) Dependency {
	return Dependency{
		Name: "redis",
		Check: func(ctx context.Context) error {
			client := redis.NewClient(&redis.Options{
				Addr:     addr,
				Password: password,
				DB:       db,
			})
			defer client.Close()
			if err := client.Ping(ctx).Err(); err != nil {
				return fmt.Errorf("redis: %w", err)
			}
			return nil
		},
	}
}

// MySQL 构造 MySQL 连通性检查。
// DSN 解析失败时只报告依赖名，避免把 DSN 写进日志。
func MySQL(dsn string) Dependency {
	return Dependency{
		Name: "mysql",
		Check: func(ctx context.Context) error {
			// WARNING S级：错误净化——driver 的 open 错误可能含 DSN，必须掩码。
			// BEGIN S_LEVEL_REFERENCE
			db, err := sql.Open("mysql", dsn)
			if err != nil {
				return fmt.Errorf("mysql: invalid DSN")
			}
			// END S_LEVEL_REFERENCE
			defer db.Close()
			if err := db.PingContext(ctx); err != nil {
				return fmt.Errorf("mysql: %w", err)
			}
			return nil
		},
	}
}

// Elasticsearch 构造 ES 连通性检查，通过 HTTP GET / 探测。
func Elasticsearch(baseURL, username, password string) Dependency {
	return Dependency{
		Name: "elasticsearch",
		Check: func(ctx context.Context) error {
			endpoint, err := url.Parse(baseURL)
			if err != nil {
				return fmt.Errorf("elasticsearch: invalid url")
			}
			endpoint.Path = "/"
			req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint.String(), nil)
			if err != nil {
				return fmt.Errorf("elasticsearch: %w", err)
			}
			if username != "" || password != "" {
				req.SetBasicAuth(username, password)
			}
			resp, err := http.DefaultClient.Do(req)
			if err != nil {
				return fmt.Errorf("elasticsearch: %w", err)
			}
			defer resp.Body.Close()
			if resp.StatusCode >= http.StatusInternalServerError {
				return fmt.Errorf("elasticsearch: status %s", resp.Status)
			}
			return nil
		},
	}
}
