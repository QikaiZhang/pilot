package tools

import (
	"context"
	"encoding/json"
	"errors"
)

// HealthDependency 是健康检查工具使用的最小探针。
// 组合根可以把 deps.Dependency 转换为该结构，工具层不需要知道 Redis/MySQL 的客户端类型。
type HealthDependency struct {
	Name  string
	Check func(context.Context) error
}

// HealthCheckResult 是模型和调用方都容易理解的健康检查结果。
type HealthCheckResult struct {
	Healthy  bool               `json:"healthy"`
	Services []DependencyStatus `json:"services"`
}

// DependencyStatus 表示一个依赖的检查结果。
type DependencyStatus struct {
	Name   string `json:"name"`
	Status string `json:"status"`
	Error  string `json:"error,omitempty"`
}

// HealthCheck 检查一组已注册的基础设施依赖。
type HealthCheck struct {
	dependencies []HealthDependency
}

// NewHealthCheck 创建健康检查工具。依赖顺序会被保留，保证输出稳定、便于审计和测试。
func NewHealthCheck(dependencies []HealthDependency) *HealthCheck {
	copyDependencies := append([]HealthDependency(nil), dependencies...)
	return &HealthCheck{dependencies: copyDependencies}
}

// Name 返回模型调用的稳定工具名。
func (h *HealthCheck) Name() string { return "health_check" }

// Description 返回模型选择工具时使用的简短说明。
func (h *HealthCheck) Description() string {
	return "检查 MySQL、Redis、Elasticsearch 等基础设施是否可用。"
}

// Parameters 返回无必填参数的 JSON Schema。
func (h *HealthCheck) Parameters() json.RawMessage {
	return json.RawMessage(`{"type":"object","properties":{}}`)
}

// Execute 执行所有探针。单个依赖失败会记录为 down，但不会跳过其他依赖。
// 这使 Agent 能得到完整的部分失败信息，而不是只看到第一个错误。
func (h *HealthCheck) Execute(ctx context.Context, input json.RawMessage) (any, error) {
	if h == nil {
		return nil, errHealthCheckUnavailable
	}
	if len(input) > 0 && string(input) != "null" {
		var params map[string]json.RawMessage
		if err := json.Unmarshal(input, &params); err != nil {
			return nil, errInvalidHealthInput
		}
	}

	result := HealthCheckResult{
		Healthy:  true,
		Services: make([]DependencyStatus, 0, len(h.dependencies)),
	}
	for _, dependency := range h.dependencies {
		status := DependencyStatus{Name: dependency.Name, Status: "up"}
		if dependency.Name == "" || dependency.Check == nil {
			status.Name = dependency.Name
			status.Status = "down"
			status.Error = "health check is not configured"
		} else if err := dependency.Check(ctx); err != nil {
			status.Status = "down"
			status.Error = err.Error()
		}
		if status.Status != "up" {
			result.Healthy = false
		}
		result.Services = append(result.Services, status)
	}
	return result, nil
}

var (
	errHealthCheckUnavailable = errors.New("health check tool is unavailable")
	errInvalidHealthInput     = errors.New("health check input is invalid")
)
