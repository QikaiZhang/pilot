// ScriptedModel 是 mock 模式下的确定性编排模型，让多代理闭环在本地无 API Key、
// 无真实模型时也能端到端运行（演示、联调、微基准共用）。
// 它按系统提示词识别调用方——分诊调用返回可解析的意图 JSON（复用规则基线的
// 关键词判定），综合调用返回引用全部证据编号的结论——因此走的是与真实模型
// 完全相同的编排路径，包括引用校验和降级分支。
package multiagent

import (
	"context"
	"errors"
	"fmt"
	"regexp"
	"strings"
	"time"

	"Pilot/internal/ai"
)

var evidenceIDPattern = regexp.MustCompile(`\[(F\d+)\]`)

// ScriptedModel 的字段是微基准与故障演练的注入口，正常运行全部零值。
type ScriptedModel struct {
	// ClassifyDelay 模拟分诊调用耗时，用于演练分类超时回退（需大于 planner 的 3s 分类超时）。
	ClassifyDelay time.Duration
	// SynthesisAnswer 非空时覆盖综合回复，用于演练"未引用证据"的降级路径。
	SynthesisAnswer string
}

// NewScriptedModel 返回零值脚本模型。
func NewScriptedModel() ScriptedModel {
	return ScriptedModel{}
}

func (m ScriptedModel) Generate(ctx context.Context, request ai.ModelRequest) (ai.ModelResponse, error) {
	if err := ctx.Err(); err != nil {
		return ai.ModelResponse{}, err
	}
	system := systemMessage(request.Messages)
	query := lastUserContent(request.Messages)

	switch system {
	case classifySystemPrompt:
		if m.ClassifyDelay > 0 {
			select {
			case <-time.After(m.ClassifyDelay):
			case <-ctx.Done():
				return ai.ModelResponse{}, ctx.Err()
			}
		}
		intent, confidence := ruleClassify(query)
		content := fmt.Sprintf(`{"intent":%q,"confidence":%v,"complexity":%q}`, intent, confidence, ruleComplexity(query))
		return scriptedResponse(content, query), nil
	case synthesisSystemPrompt:
		if m.SynthesisAnswer != "" {
			return scriptedResponse(m.SynthesisAnswer, query), nil
		}
		ids := evidenceIDPattern.FindAllStringSubmatch(query, -1)
		return scriptedResponse(scriptedSynthesis(ids), query), nil
	default:
		return scriptedResponse("已收到："+query, query), nil
	}
}

// Stream 供接口满足：编排路径只使用 Generate，流式属于聊天服务的能力。
func (m ScriptedModel) Stream(context.Context, ai.ModelRequest) (ai.TokenStream, error) {
	return nil, errors.New("scripted model does not support streaming")
}

// scriptedSynthesis 产出引用全部证据编号的确定性结论；没有编号时给无引用
// 的兜底文本（编排器会按"引用缺失"降级，同样是一条可验证路径）。
func scriptedSynthesis(matches [][]string) string {
	if len(matches) == 0 {
		return "证据内容不足以给出结论，建议补充运行时数据。"
	}
	citations := make([]string, 0, len(matches))
	seen := make(map[string]struct{}, len(matches))
	for _, match := range matches {
		id := match[1]
		if _, exists := seen[id]; exists {
			continue
		}
		seen[id] = struct{}{}
		citations = append(citations, "["+id+"]")
	}
	return fmt.Sprintf("综合结论：已交叉核对 %d 条证据 %s，初步判断与证据一致。"+
		"请按知识库处置步骤执行，并持续观察核心指标是否恢复；若未恢复，升级为复杂故障重新排查。",
		len(citations), strings.Join(citations, " "))
}

func scriptedResponse(content, query string) ai.ModelResponse {
	return ai.ModelResponse{
		Message: ai.Message{Role: ai.RoleAssistant, Content: content},
		Usage:   ai.Usage{InputTokens: len([]rune(query)), OutputTokens: len([]rune(content))},
	}
}

func systemMessage(messages []ai.Message) string {
	for _, message := range messages {
		if message.Role == ai.RoleSystem {
			return message.Content
		}
	}
	return ""
}

func lastUserContent(messages []ai.Message) string {
	for index := len(messages) - 1; index >= 0; index-- {
		if messages[index].Role == ai.RoleUser {
			return messages[index].Content
		}
	}
	return ""
}
