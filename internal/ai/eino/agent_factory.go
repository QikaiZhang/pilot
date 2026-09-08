package eino

import (
	"context"
	"errors"
	"fmt"

	projectagent "Pilot/internal/agent"
	projecttools "Pilot/internal/tools"

	"github.com/cloudwego/eino/components/model"
	"github.com/cloudwego/eino/components/tool"
	"github.com/cloudwego/eino/compose"
	"github.com/cloudwego/eino/flow/agent/react"
)

// NewReActAgent 根据项目策略创建 Eino ReAct Agent。
// 它在组合根调用：模型、白名单工具和最大步数在这里一次性固定。
func NewReActAgent(
	ctx context.Context,
	chatModel model.ToolCallingChatModel,
	registry *projecttools.Registry,
	policy projectagent.AgentPolicy,
) (*react.Agent, error) {
	if chatModel == nil {
		return nil, ErrToolCallingModelRequired
	}
	if registry == nil {
		return nil, ErrToolRegistryRequired
	}
	if err := policy.Validate(); err != nil {
		return nil, err
	}
	policy = policy.Normalize()

	einoTools, err := newEinoTools(registry, policy)
	if err != nil {
		return nil, err
	}
	agent, err := react.NewAgent(ctx, &react.AgentConfig{
		ToolCallingModel: chatModel,
		MaxStep:          policy.Execution.MaxSteps,
		ToolsConfig: compose.ToolsNodeConfig{
			Tools:               einoTools,
			ExecuteSequentially: true,
			ToolCallMiddlewares: []compose.ToolMiddleware{newPolicyMiddleware(policy)},
		},
		GraphName: "PilotReActAgent",
	})
	if err != nil {
		return nil, fmt.Errorf("create eino react agent: %w", err)
	}
	return agent, nil
}

// newEinoTools 先通过项目 Registry 应用白名单，再逐个适配为 Eino 工具。
// 第一版明确顺序执行，避免模型同轮调用之间的隐式依赖被并行打乱。
func newEinoTools(registry *projecttools.Registry, policy projectagent.AgentPolicy) ([]tool.BaseTool, error) {
	allowedTools, err := registry.ListAllowed(policy.AllowedTools)
	if err != nil {
		return nil, fmt.Errorf("resolve allowed tools: %w", err)
	}
	einoTools := make([]tool.BaseTool, 0, len(allowedTools))
	for _, projectTool := range allowedTools {
		adapter, err := NewToolAdapter(projectTool)
		if err != nil {
			return nil, fmt.Errorf("adapt tool %q: %w", projectTool.Name(), err)
		}
		einoTools = append(einoTools, adapter)
	}
	return einoTools, nil
}

var (
	ErrToolCallingModelRequired = errors.New("eino tool-calling model is required")
	ErrToolRegistryRequired     = errors.New("eino tool registry is required")
)
