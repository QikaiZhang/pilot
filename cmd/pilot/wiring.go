package main

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"

	projectagent "Pilot/internal/agent"
	"Pilot/internal/ai"
	einoadapter "Pilot/internal/ai/eino"
	"Pilot/internal/ai/embedder"
	"Pilot/internal/ai/retriever"
	"Pilot/internal/chat"
	"Pilot/internal/deps"
	"Pilot/internal/memory"
	"Pilot/internal/tools"
	"Pilot/pkg/configutil"

	"github.com/cloudwego/eino/components/model"
	_ "github.com/go-sql-driver/mysql"
	"github.com/redis/go-redis/v9"
)

type runtime struct {
	chat      *chat.Service
	agent     *chat.AgentService
	knowledge *retriever.Ingestor
	close     func() error
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
	rag, err := buildRAG(ctx, cfg)
	if err != nil {
		_ = closeResources()
		return nil, fmt.Errorf("create rag runtime: %w", err)
	}

	configuredAgent, err := buildAgent(ctx, cfg, model, rag.retriever)
	if err != nil {
		_ = closeResources()
		return nil, fmt.Errorf("create agent: %w", err)
	}

	var agentService *chat.AgentService
	if configuredAgent != nil {
		agentService, err = chat.NewAgentService(memoryService, configuredAgent, chat.Config{
			HistoryLimit: cfg.Redis.MaxMessages,
			SystemPrompt: "你是 Pilot 的运维助手，请给出简洁、可执行且安全的建议。",
		})
		if err != nil {
			_ = closeResources()
			return nil, fmt.Errorf("create agent service: %w", err)
		}
	}

	return &runtime{chat: chatService, agent: agentService, knowledge: rag.ingestor, close: closeResources}, nil
}

type ragRuntime struct {
	retriever ai.Retriever
	ingestor  *retriever.Ingestor
}

func buildRAG(ctx context.Context, cfg configutil.Config) (*ragRuntime, error) {
	esConfig := retriever.ESConfig{Addresses: []string{cfg.ES.URL}, Username: cfg.ES.Username, Password: cfg.ES.Password, KnowledgeIndex: cfg.ES.KnowledgeIndex}
	em, err := embedder.NewMock(cfg.Embedding.Dim)
	if err != nil {
		return nil, fmt.Errorf("create rag embedder: %w", err)
	}
	store, err := retriever.NewESStore(ctx, esConfig, em.Dim())
	if err != nil {
		return nil, fmt.Errorf("create rag store: %w", err)
	}
	ingestor, err := retriever.NewIngestor(store, em, "mock", 1800, 200)
	if err != nil {
		return nil, fmt.Errorf("create rag ingestor: %w", err)
	}
	client, err := retriever.NewESClient(esConfig)
	if err != nil {
		return nil, fmt.Errorf("create rag search client: %w", err)
	}
	searcher, err := retriever.NewESRetriever(client, cfg.ES.KnowledgeIndex, em, 5, 5*time.Second)
	if err != nil {
		return nil, fmt.Errorf("create rag retriever: %w", err)
	}
	return &ragRuntime{retriever: searcher, ingestor: ingestor}, nil
}

func buildAgent(ctx context.Context, cfg configutil.Config, chatModel ai.ChatModel, knowledgeRetriever ai.Retriever) (projectagent.Runner, error) {
	if strings.ToLower(strings.TrimSpace(cfg.LLM.Mode)) == "mock" || strings.TrimSpace(cfg.LLM.Mode) == "" {
		return nil, nil
	}
	provider, ok := chatModel.(interface {
		ToolCallingModel() (model.ToolCallingChatModel, error)
	})
	if !ok {
		return nil, fmt.Errorf("configured chat model does not support tool calling")
	}
	toolCallingModel, err := provider.ToolCallingModel()
	if err != nil {
		return nil, err
	}
	checkers := []deps.Dependency{
		deps.Redis(cfg.Redis.Addr, cfg.Redis.Password, cfg.Redis.DB),
		deps.MySQL(cfg.MySQL.DSN),
		deps.Elasticsearch(cfg.ES.URL, cfg.ES.Username, cfg.ES.Password),
	}
	healthDependencies := make([]tools.HealthDependency, 0, len(checkers))
	for _, checker := range checkers {
		healthDependencies = append(healthDependencies, tools.HealthDependency{Name: checker.Name, Check: checker.Check})
	}
	registryTools := []tools.Tool{tools.NewHealthCheck(healthDependencies)}
	if knowledgeRetriever != nil {
		registryTools = append(registryTools, tools.NewKnowledgeSearch(knowledgeRetriever))
	}
	registry, err := tools.NewRegistry(registryTools...)
	if err != nil {
		return nil, err
	}
	policy := projectagent.AgentPolicy{
		Execution: projectagent.ExecutionPolicy{
			MaxSteps: 4,
			Timeout:  30 * time.Second,
		},
		Budget: projectagent.BudgetPolicy{
			MaxToolCalls:      8,
			MaxArgumentsBytes: 64 << 10,
		},
		Repeat:       projectagent.RepeatCallPolicy{MaxConsecutiveFailures: 2},
		AllowedTools: []string{"health_check", "knowledge_search"},
		Fallback:     projectagent.FallbackPolicy{Mode: projectagent.FallbackReturnError},
	}
	einoAgent, err := einoadapter.NewReActAgent(ctx, toolCallingModel, registry, policy)
	if err != nil {
		return nil, err
	}
	einoRunner, err := einoadapter.NewEinoRunner(einoAgent, policy)
	if err != nil {
		return nil, err
	}
	// 请求级超时、取消语义和 Fallback 由 PolicyAwareRunner 收口；
	// EinoRunner 只负责 ReAct 能力和工具调用级中间件。
	return projectagent.NewPolicyAwareRunner(einoRunner, policy)
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
