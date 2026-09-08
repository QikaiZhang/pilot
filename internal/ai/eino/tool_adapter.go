package eino

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	projecttools "Pilot/internal/tools"

	"github.com/cloudwego/eino/components/tool"
	"github.com/cloudwego/eino/schema"
	"github.com/eino-contrib/jsonschema"
)

// ToolAdapter 将项目工具适配为 Eino 可注册和执行的 InvokableTool。
// Eino 依赖只停留在该适配层，项目工具本身不感知具体 Agent 框架。
type ToolAdapter struct {
	tool projecttools.Tool
}

// 关键的一行不满足则编译不通过
// _代表这个变凉了我直接不使用，右边是对*ToolAdapter 的一个空实现
var _ tool.InvokableTool = (*ToolAdapter)(nil)

// NewToolAdapter 创建 Eino 工具适配器，并在启动时校验工具元数据。
func NewToolAdapter(projectTool projecttools.Tool) (*ToolAdapter, error) {
	if projectTool == nil {
		return nil, ErrInvalidToolAdapter
	}
	if strings.TrimSpace(projectTool.Name()) == "" {
		return nil, ErrInvalidToolAdapter
	}
	parameters := projectTool.Parameters()
	if !json.Valid(parameters) {
		return nil, fmt.Errorf("%w: invalid parameters schema", ErrInvalidToolAdapter)
	}
	return &ToolAdapter{tool: projectTool}, nil
}

// Info 将项目工具的描述转换为 Eino ToolInfo，供模型决定是否调用。
func (a *ToolAdapter) Info(context.Context) (*schema.ToolInfo, error) {
	if a == nil || a.tool == nil {
		return nil, ErrInvalidToolAdapter
	}
	var parameters jsonschema.Schema
	if err := json.Unmarshal(a.tool.Parameters(), &parameters); err != nil {
		return nil, fmt.Errorf("decode %s parameters schema: %w", a.tool.Name(), err)
	}
	return &schema.ToolInfo{
		Name:        a.tool.Name(),
		Desc:        a.tool.Description(),
		ParamsOneOf: schema.NewParamsOneOfByJSONSchema(&parameters),
	}, nil
}

// InvokableRun 把 Eino 的 JSON 参数交给项目工具，并将结构化结果编码回 JSON。
func (a *ToolAdapter) InvokableRun(ctx context.Context, argumentsInJSON string, _ ...tool.Option) (string, error) {
	if a == nil || a.tool == nil {
		return "", ErrInvalidToolAdapter
	}
	if !json.Valid([]byte(argumentsInJSON)) {
		return "", fmt.Errorf("%w: invalid arguments JSON", ErrInvalidToolArguments)
	}
	result, err := a.tool.Execute(ctx, json.RawMessage(argumentsInJSON))
	if err != nil {
		return "", fmt.Errorf("execute %s: %w", a.tool.Name(), err)
	}
	encoded, err := json.Marshal(result)
	if err != nil {
		return "", fmt.Errorf("encode %s result: %w", a.tool.Name(), err)
	}
	return string(encoded), nil
}

var (
	ErrInvalidToolAdapter   = errors.New("eino tool adapter is invalid")
	ErrInvalidToolArguments = errors.New("eino tool arguments are invalid")
)
