// Package prompt 负责把检索到的知识片段组装成可注入模型上下文的文本。
// 它把每个 Chunk 规整成带来源标注的引用块，便于模型回答时说明依据，
// 也便于上层对回答做证据追溯。
package prompt

import (
	"fmt"
	"strings"

	"Pilot/internal/ai"
)

// BuildKnowledgeContext 把命中的知识片段拼接成一段可直接注入 system 消息的上下文。
// 每个片段输出为「引用块」，含标题、来源与正文；无片段时返回提示知识库为空。
func BuildKnowledgeContext(chunks []ai.Chunk) string {
	if len(chunks) == 0 {
		return "知识库未检索到相关内容，请基于通用知识回答，并说明未使用检索结果。"
	}

	var builder strings.Builder
	builder.WriteString("以下是知识库检索到的相关内容，请优先据此回答，并标注依据片段：\n")
	for i, chunk := range chunks {
		fmt.Fprintf(&builder, "[%d] %s", i+1, chunk.Title)
		if chunk.Source != "" {
			builder.WriteString("（来源：" + chunk.Source + "）")
		}
		builder.WriteString("\n")
		builder.WriteString(chunk.Content)
		builder.WriteString("\n\n")
	}
	return strings.TrimRight(builder.String(), "\n")
}
