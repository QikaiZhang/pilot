package agent

import (
	"context"
	"errors"
)

// PolicyAwareRunner 是请求级的策略收口层，与具体框架无关。
// 三种执行粒度的分工：
//
//	工具调用级：参数预算、白名单预算、重复失败保护，由具体 Runner 内部的
//	            工具中间件执行（如 Eino 的 ToolCallMiddlewares）；
//	模型轮次级：决策轮数与 step 计数，由 Agent 循环本身执行（Eino MaxStep）；
//	请求级：    总超时、取消/超时的稳定语义和 FallbackPolicy，由本层执行。
//
// 具体 Runner 可以从本层派生的 context 再次派生自己的子 deadline；两层使用
// 同一策略时长时，实际生效的是先到的那一层，语义一致。
type PolicyAwareRunner struct {
	inner  Runner
	policy AgentPolicy
}

var _ Runner = (*PolicyAwareRunner)(nil)

// NewPolicyAwareRunner 包装任意 Runner 并在构造期校验策略。
// 策略从服务端配置传入，不从用户请求读取。
func NewPolicyAwareRunner(inner Runner, policy AgentPolicy) (*PolicyAwareRunner, error) {
	if inner == nil {
		return nil, ErrInvalidRunner
	}
	if err := policy.Validate(); err != nil {
		return nil, err
	}
	return &PolicyAwareRunner{inner: inner, policy: policy.Normalize()}, nil
}

// Run 执行一次请求级受控的 Agent 调用。
// 超时和取消优先于普通错误：即使内部 Runner 返回了工具错误，只要请求的
// 生命周期先结束，对外就返回稳定的超时/取消语义，并保留已完成的工具审计。
func (r *PolicyAwareRunner) Run(ctx context.Context, request AgentRequest) (AgentResponse, error) {
	if r == nil || r.inner == nil {
		return AgentResponse{}, ErrInvalidRunner
	}
	if err := request.Validate(); err != nil {
		return AgentResponse{}, err
	}
	runCtx, cancel := r.policy.Context(ctx)
	defer cancel()

	response, err := r.inner.Run(runCtx, request)
	if err != nil {
		return r.resolveFailure(runCtx, response, err)
	}
	return response, nil
}

// resolveFailure 区分生命周期失败与普通执行失败。
// 超时检查必须同时看错误链和 runCtx 自身：内部 Runner 可能把
// DeadlineExceeded 包在框架错误里，也可能直接透传依赖错误。
func (r *PolicyAwareRunner) resolveFailure(runCtx context.Context, response AgentResponse, cause error) (AgentResponse, error) {
	if errors.Is(cause, context.DeadlineExceeded) || runCtx.Err() == context.DeadlineExceeded {
		response.Limitations = append(response.Limitations, "agent execution timed out")
		return response, ErrAgentTimeout
	}
	if errors.Is(cause, context.Canceled) || runCtx.Err() == context.Canceled {
		response.Limitations = append(response.Limitations, "agent execution was cancelled")
		return response, ErrAgentCancelled
	}
	return r.policy.ResolveFallback(response, cause, "agent execution failed")
}
