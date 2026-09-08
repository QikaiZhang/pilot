package eino

import (
	"context"
	"errors"
	"fmt"
	"io"
	"strings"
	"time"

	projectai "Pilot/internal/ai"

	openai "github.com/cloudwego/eino-ext/components/model/openai"
	einomodel "github.com/cloudwego/eino/components/model"
	"github.com/cloudwego/eino/schema"
)

var (
	ErrAPIKeyRequired = errors.New("eino api key is required")
	ErrModelRequired  = errors.New("eino model name is required")
)

// Config 保存创建 Eino OpenAI-compatible 客户端所需的供应商配置。
// 它与 HTTP 请求分开，避免调用方直接控制模型运行参数。
type Config struct {
	APIKey      string
	BaseURL     string
	Model       string
	Temperature float32
	MaxTokens   int
	Timeout     time.Duration
}

// ChatModel 将 Eino 的 OpenAI-compatible 实现适配为项目的
// 供应商无关 ai.ChatModel 接口。
type ChatModel struct {
	model *openai.ChatModel
}

var _ projectai.ChatModel = (*ChatModel)(nil)

// ToolCallingModel 暴露同一底层客户端的工具调用能力，供 Eino ReAct 组装使用。
// 普通 ChatModel 接口仍保持不变，调用方只有在明确需要 Agent 时才使用该能力。
func (m *ChatModel) ToolCallingModel() (einomodel.ToolCallingChatModel, error) {
	if m == nil || m.model == nil {
		return nil, errors.New("eino tool-calling model is not initialized")
	}
	return m.model, nil
}

// NewChatModel 创建一个 ChatModel 实例。
func NewChatModel(ctx context.Context, config Config) (*ChatModel, error) {
	if strings.TrimSpace(config.APIKey) == "" {
		return nil, ErrAPIKeyRequired
	}
	if strings.TrimSpace(config.Model) == "" {
		return nil, ErrModelRequired
	}
	if config.MaxTokens < 0 {
		return nil, fmt.Errorf("max tokens must not be negative")
	}

	temperature := config.Temperature
	modelConfig := &openai.ChatModelConfig{
		APIKey:      config.APIKey,
		BaseURL:     normalizeBaseURL(config.BaseURL),
		Model:       config.Model,
		Temperature: &temperature,
		Timeout:     config.Timeout,
	}
	if config.MaxTokens > 0 {
		modelConfig.MaxTokens = &config.MaxTokens
	}

	model, err := openai.NewChatModel(ctx, modelConfig)
	if err != nil {
		return nil, fmt.Errorf("create eino openai chat model: %w", err)
	}
	return &ChatModel{model: model}, nil
}

// normalizeBaseURL 接受供应商常见的两种填写方式，统一成 SDK 需要的基础地址。
func normalizeBaseURL(baseURL string) string {
	baseURL = strings.TrimRight(strings.TrimSpace(baseURL), "/")
	return strings.TrimSuffix(baseURL, "/chat/completions")
}

func (m *ChatModel) Generate(ctx context.Context, request projectai.ModelRequest) (projectai.ModelResponse, error) {
	if m == nil || m.model == nil {
		return projectai.ModelResponse{}, errors.New("eino chat model is not initialized")
	}
	// 将项目 ai 层的消息转换为 Eino 能使用的消息。
	messages, err := toEinoMessages(request.Messages)
	if err != nil {
		return projectai.ModelResponse{}, err
	}
	response, err := m.model.Generate(ctx, messages, requestOptions(request.Options)...)
	if err != nil {
		return projectai.ModelResponse{}, fmt.Errorf("eino generate: %w", err)
	}
	return fromEinoResponse(response)
}

func (m *ChatModel) Stream(ctx context.Context, request projectai.ModelRequest) (projectai.TokenStream, error) {
	if m == nil || m.model == nil {
		return nil, errors.New("eino chat model is not initialized")
	}
	messages, err := toEinoMessages(request.Messages)
	if err != nil {
		return nil, err
	}
	// Eino 返回 schema.StreamReader，由适配器包装成项目自己的 TokenStream。
	stream, err := m.model.Stream(ctx, messages, requestOptions(request.Options)...)
	if err != nil {
		return nil, fmt.Errorf("eino stream: %w", err)
	}
	return &tokenStream{reader: stream}, nil
}

func requestOptions(options projectai.ModelOptions) []einomodel.Option {
	result := make([]einomodel.Option, 0, 3)
	if options.Model != "" {
		result = append(result, einomodel.WithModel(options.Model))
	}
	if options.Temperature != 0 {
		result = append(result, einomodel.WithTemperature(options.Temperature))
	}
	if options.MaxTokens > 0 {
		result = append(result, einomodel.WithMaxTokens(options.MaxTokens))
	}
	return result
}

func fromEinoResponse(response *schema.Message) (projectai.ModelResponse, error) {
	if response == nil {
		return projectai.ModelResponse{}, errors.New("eino returned an empty response")
	}
	return projectai.ModelResponse{
		Message: projectai.Message{
			Role:    projectai.Role(response.Role),
			Content: response.Content,
		},
		Usage: usageFromResponse(response),
	}, nil
}

func usageFromResponse(message *schema.Message) projectai.Usage {
	if message == nil || message.ResponseMeta == nil || message.ResponseMeta.Usage == nil {
		return projectai.Usage{}
	}
	usage := message.ResponseMeta.Usage
	return projectai.Usage{
		InputTokens:  usage.PromptTokens,
		OutputTokens: usage.CompletionTokens,
	}
}

type tokenStream struct {
	reader *schema.StreamReader[*schema.Message]
}

func (s *tokenStream) Recv() (projectai.ModelChunk, error) {
	if s == nil || s.reader == nil {
		return projectai.ModelChunk{}, errors.New("eino token stream is not initialized")
	}
	// 调用 Eino 提供的 schema.StreamReader.Recv 读取下一个消息片段。
	message, err := s.reader.Recv()
	if err != nil {
		if errors.Is(err, io.EOF) {
			return projectai.ModelChunk{}, io.EOF
		}
		return projectai.ModelChunk{}, fmt.Errorf("receive eino stream chunk: %w", err)
	}
	if message == nil {
		return projectai.ModelChunk{}, errors.New("eino returned an empty stream chunk")
	}
	// 将 Eino 的原始 Message 响应转换成项目 ai 层使用的 chunk。
	chunk := projectai.ModelChunk{Delta: message.Content}
	if message.ResponseMeta != nil {
		chunk.FinishReason = message.ResponseMeta.FinishReason
		if message.ResponseMeta.Usage != nil {
			usage := usageFromResponse(message)
			chunk.Usage = &usage
		}
	}
	return chunk, nil
}

func (s *tokenStream) Close() error {
	if s == nil || s.reader == nil {
		return nil
	}
	s.reader.Close()
	return nil
}

// toEinoMessages 将项目 ai 层的 Message 转换为 Eino 的 Message。
func toEinoMessages(messages []projectai.Message) ([]*schema.Message, error) {
	// 长度从 0 开始、容量预留为消息数量，避免结果中出现默认零值元素。
	converted := make([]*schema.Message, 0, len(messages))
	for index, message := range messages {
		if !message.Role.Valid() {
			return nil, fmt.Errorf("message %d has invalid role %q", index, message.Role)
		}
		if strings.TrimSpace(message.Content) == "" {
			return nil, fmt.Errorf("message %d has empty content", index)
		}
		converted = append(converted, &schema.Message{
			Role:    schema.RoleType(message.Role),
			Content: message.Content,
		})
	}
	return converted, nil
}
