package agent

import (
	"errors"

	"Pilot/internal/ai"
	"Pilot/internal/tools"
)

// RunnerService 是 Agent 的组合对象。
// 它持有业务策略和基础能力，Run 只接收本次请求，不从请求中读取执行策略。
type RunnerService struct {
	model    ai.ToolCallingChatModel
	registry *tools.Registry
	policy   AgentPolicy
}

// NewRunner 创建 Agent Runner，并在启动阶段校验依赖和策略。
// 早失败可以把配置问题留在组合根，而不是等到线上请求才暴露。
func NewRunner(model ai.ToolCallingChatModel, registry *tools.Registry, policy AgentPolicy) (*RunnerService, error) {
	if model == nil || registry == nil {
		return nil, ErrInvalidRunner
	}
	if err := policy.Validate(); err != nil {
		return nil, err
	}
	return &RunnerService{model: model, registry: registry, policy: policy}, nil
}

var ErrInvalidRunner = errors.New("agent runner is invalid")
