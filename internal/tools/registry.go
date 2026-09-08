package tools

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"Pilot/internal/ai"
)

// Registry 保存 Agent 可以使用的工具。
// map 只在内部使用；对外统一通过方法读取，保证名称校验和白名单规则集中。
type Registry struct {
	tools map[string]Tool
	order []string
}

// NewRegistry 创建工具注册表。
// 注册阶段失败比运行时才发现重名或空名称更容易定位，因此这里拒绝无效配置。
func NewRegistry(toolList ...Tool) (*Registry, error) {
	registry := &Registry{
		tools: make(map[string]Tool, len(toolList)),
		order: make([]string, 0, len(toolList)),
	}
	for _, tool := range toolList {
		if tool == nil {
			return nil, ErrInvalidTool
		}
		name := strings.TrimSpace(tool.Name())
		if name == "" {
			return nil, ErrInvalidTool
		}
		if _, exists := registry.tools[name]; exists {
			return nil, fmt.Errorf("%w: %s", ErrDuplicateTool, name)
		}
		registry.tools[name] = tool
		registry.order = append(registry.order, name)
	}
	return registry, nil
}

// Get 按模型返回的工具名查找工具。
func (r *Registry) Get(name string) (Tool, error) {
	if r == nil {
		return nil, ErrToolNotFound
	}
	tool, ok := r.tools[strings.TrimSpace(name)]
	if !ok {
		return nil, fmt.Errorf("%w: %s", ErrToolNotFound, name)
	}
	return tool, nil
}

// ListAllowed 返回白名单内的工具。
// allowed 为空时按 fail-closed 处理，即不暴露任何工具；顺序遵循注册顺序。
// 白名单包含未注册名称时返回错误，避免策略拼写错误被静默忽略。
func (r *Registry) ListAllowed(allowed []string) ([]Tool, error) {
	if r == nil {
		return nil, ErrToolNotFound
	}
	allowedSet := make(map[string]struct{}, len(allowed))
	for _, name := range allowed {
		name = strings.TrimSpace(name)
		if name == "" {
			return nil, ErrInvalidTool
		}
		if _, ok := r.tools[name]; !ok {
			return nil, fmt.Errorf("%w: %s", ErrToolNotFound, name)
		}
		allowedSet[name] = struct{}{}
	}

	result := make([]Tool, 0, len(allowedSet))
	for _, name := range r.order {
		if _, ok := allowedSet[name]; ok {
			result = append(result, r.tools[name])
		}
	}
	return result, nil
}

// Definitions 将白名单内工具转换为模型可读的项目契约。
func (r *Registry) Definitions(allowed []string) ([]ai.ToolDefinition, error) {
	tools, err := r.ListAllowed(allowed)
	if err != nil {
		return nil, err
	}
	definitions := make([]ai.ToolDefinition, 0, len(tools))
	for _, tool := range tools {
		parameters := tool.Parameters()
		if !json.Valid(parameters) {
			return nil, fmt.Errorf("%w: %s parameters", ErrInvalidTool, tool.Name())
		}
		definitions = append(definitions, ai.ToolDefinition{
			Name:        tool.Name(),
			Description: tool.Description(),
			Parameters:  parameters,
		})
	}
	return definitions, nil
}

var (
	ErrInvalidTool   = errors.New("tool is invalid")
	ErrDuplicateTool = errors.New("tool is duplicated")
	ErrToolNotFound  = errors.New("tool is not registered")
)
