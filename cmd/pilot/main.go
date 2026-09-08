// Pilot 服务入口：加载配置、检查核心依赖、启动 HTTP 服务并优雅关闭。
package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"Pilot/internal/deps"
	"Pilot/internal/handler"
	"Pilot/pkg/configutil"
	"Pilot/pkg/envloader"
)

const (
	startupCheckTimeout = 10 * time.Second
	shutdownTimeout     = 10 * time.Second
)

func main() {
	logger := slog.New(slog.NewJSONHandler(os.Stdout, nil))
	slog.SetDefault(logger)

	if err := run(); err != nil {
		slog.Error("pilot exited with error", "error", err)
		os.Exit(1)
	}
}

func run() error {
	// .env 缺失不视为错误；已有环境变量优先。
	_ = envloader.Load(".env")
	//加载 env
	cfg := configutil.Load()
	slog.Info("config loaded", "env", cfg.App.Env, "addr", cfg.App.Addr)

	// 启动期核心依赖检查：失败即退出，错误只含依赖名。
	checkers := []deps.Dependency{
		deps.Redis(cfg.Redis.Addr, cfg.Redis.Password, cfg.Redis.DB),
		deps.MySQL(cfg.MySQL.DSN),
		deps.Elasticsearch(cfg.ES.URL, cfg.ES.Username, cfg.ES.Password),
	}
	startupCtx, cancelStartup := context.WithTimeout(context.Background(), startupCheckTimeout)
	defer cancelStartup()
	results := deps.CheckAll(startupCtx, checkers)
	var failed []string
	for _, r := range results {
		if r.OK {
			slog.Info("dependency up", "name", r.Name, "latency_ms", r.LatencyMS)
			continue
		}
		slog.Error("dependency down", "name", r.Name, "error", r.Err)
		failed = append(failed, r.Err.Error())
	}
	if len(failed) > 0 {
		return fmt.Errorf("startup dependency check failed: %s", strings.Join(failed, "; "))
	}

	runtimeCtx, cancelRuntime := context.WithTimeout(context.Background(), startupCheckTimeout)
	rt, err := buildRuntime(runtimeCtx, cfg)
	cancelRuntime()
	if err != nil {
		return fmt.Errorf("build application runtime: %w", err)
	}
	defer func() {
		if err := rt.close(); err != nil {
			slog.Warn("close application resources failed", "error", err)
		}
	}()

	mux := http.NewServeMux()
	health := handler.NewHealthHandler(checkers)
	mux.HandleFunc("GET /api/v1/health/live", health.Live)
	mux.HandleFunc("GET /api/v1/health/ready", health.Ready)
	chatHandler := handler.NewChatHandler(rt.chat)
	mux.HandleFunc("POST /api/v1/chat", chatHandler.Generate)
	mux.HandleFunc("POST /api/v1/chat/stream", chatHandler.Stream)
	agentHandler := handler.NewAgentHandler(rt.agent)
	mux.HandleFunc("POST /api/v1/agent/chat", agentHandler.Run)
	knowledgeHandler := handler.NewKnowledgeHandler(rt.knowledge)
	mux.HandleFunc("POST /api/v1/knowledge/documents", knowledgeHandler.Upload)

	srv := &http.Server{
		Addr:              cfg.App.Addr,
		Handler:           mux,
		ReadHeaderTimeout: 5 * time.Second,
	}

	// 优雅关闭：收到 SIGINT/SIGTERM 后停止接收新请求，等待存量请求结束。
	// WARNING S级核心逻辑：理解后必须删除本区域，并从零独立重写，禁止直接复用。
	// 删除范围：从 BEGIN S_LEVEL_REFERENCE 到 END S_LEVEL_REFERENCE。
	// BEGIN S_LEVEL_REFERENCE
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	errCh := make(chan error, 1)
	go func() {
		slog.Info("http server listening", "addr", cfg.App.Addr)
		errCh <- srv.ListenAndServe()
	}()

	select {
	case err := <-errCh:
		if errors.Is(err, http.ErrServerClosed) {
			return nil
		}
		return fmt.Errorf("http server: %w", err)
	case <-ctx.Done():
		slog.Info("shutdown signal received")
	}

	shutdownCtx, cancel := context.WithTimeout(context.Background(), shutdownTimeout)
	defer cancel()
	if err := srv.Shutdown(shutdownCtx); err != nil {
		return fmt.Errorf("graceful shutdown: %w", err)
	}
	slog.Info("server stopped cleanly")
	// END S_LEVEL_REFERENCE
	return nil
}
