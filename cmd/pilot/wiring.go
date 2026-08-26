package main

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"

	"Pilot/internal/ai"
	einoadapter "Pilot/internal/ai/eino"
	"Pilot/internal/chat"
	"Pilot/internal/memory"
	"Pilot/pkg/configutil"

	_ "github.com/go-sql-driver/mysql"
	"github.com/redis/go-redis/v9"
)

type runtime struct {
	chat  *chat.Service
	close func() error
}

func buildRuntime(ctx context.Context, cfg configutil.Config) (*runtime, error) {
	db, err := sql.Open("mysql", cfg.MySQL.DSN)
	if err != nil {
		return nil, fmt.Errorf("open mysql: %w", err)
	}
	redisClient := redis.NewClient(&redis.Options{
		Addr:     cfg.Redis.Addr,
		Password: cfg.Redis.Password,
		DB:       cfg.Redis.DB,
	})
	closeResources := func() error {
		redisErr := redisClient.Close()
		dbErr := db.Close()
		return errors.Join(redisErr, dbErr)
	}

	history, err := memory.NewMySQLStore(db)
	if err != nil {
		_ = closeResources()
		return nil, fmt.Errorf("create mysql memory store: %w", err)
	}
	recent, err := memory.NewRedisStore(redisClient)
	if err != nil {
		_ = closeResources()
		return nil, fmt.Errorf("create redis memory store: %w", err)
	}
	memoryService, err := memory.NewService(history, recent, memory.ServiceConfig{
		RecentLimit: cfg.Redis.MaxMessages,
		RecentTTL:   cfg.Redis.MemoryTTL,
	})
	if err != nil {
		_ = closeResources()
		return nil, fmt.Errorf("create memory service: %w", err)
	}

	model, err := buildChatModel(ctx, cfg.LLM)
	if err != nil {
		_ = closeResources()
		return nil, err
	}
	chatService, err := chat.NewService(memoryService, model, chat.Config{
		HistoryLimit: cfg.Redis.MaxMessages,
		SystemPrompt: "你是 Pilot 的运维助手，请给出简洁、可执行且安全的建议。",
	})
	if err != nil {
		_ = closeResources()
		return nil, fmt.Errorf("create chat service: %w", err)
	}

	return &runtime{chat: chatService, close: closeResources}, nil
}

func buildChatModel(ctx context.Context, cfg configutil.LLMConfig) (ai.ChatModel, error) {
	switch strings.ToLower(strings.TrimSpace(cfg.Mode)) {
	case "", "mock":
		return ai.MockChatModel{}, nil
	case "eino", "openai":
		model, err := einoadapter.NewChatModel(ctx, einoadapter.Config{
			APIKey:      cfg.APIKey,
			BaseURL:     cfg.BaseURL,
			Model:       cfg.Model,
			Temperature: cfg.Temperature,
			MaxTokens:   cfg.MaxTokens,
			Timeout:     cfg.Timeout,
		})
		if err != nil {
			return nil, fmt.Errorf("create configured chat model: %w", err)
		}
		return model, nil
	default:
		return nil, fmt.Errorf("unsupported LLM_MODE %q", cfg.Mode)
	}
}
