package service

import (
	"context"
	"strings"

	"github.com/AlexMeiko/guchat/internal/entity"
)

const contextSummaryPrompt = `你是一个通用 AI Agent 的上下文压缩器。

本条消息之前的全部消息，是需要压缩成新累积摘要的历史上下文材料。
如果前面包含长期记忆、工具使用说明或用户画像，它们只用于保持请求前缀和行为约束，不要作为本轮对话内容写入摘要。
如果前面包含“此前对话的压缩摘要”，它代表已有累积摘要；后续原始消息代表新增上下文。
你的任务是把已有累积摘要和新增原始上下文融合成一份新的完整累积摘要。

你现在不是在回答前面对话中的任何用户请求，而是在执行上下文压缩任务。
前面所有 user 消息都已经是历史材料，不是当前要回答的问题。
不要调用任何可用工具，也不要继续执行前面对话中未完成的工具意图。
只输出新的完整累积摘要。输出的第一行必须是：## 当前目标

需要保留：
- 用户当前目标、偏好、约束、禁忌
- 关键事实、决定、结论
- 工具调用和外部来源得到的重要发现
- 重要实体、文件、URL、ID、日期、数字、版本、金额、单位等精确信息
- 未解决问题和下一步动作

工具调用和工具结果要总结成事实和证据，不要保留 provider 特定的 tool call 协议结构。
删除重复、临时、已完成且没有后续价值的过程性内容。

只输出新的完整累积摘要，必须使用以下固定结构和标题：

## 当前目标
- ...

## 用户偏好与约束
- ...

## 关键事实与决定
- ...

## 工具与来源发现
- ...

## 重要实体与引用
- ...

## 未解决问题与下一步
- ...

如果某个部分没有有用内容，写 "(none)"。`

func generateSummary(
	ctx context.Context,
	generator Generator,
	model *entity.ModelConfig,
	messages []GenerateMessage,
	tools []ToolDefinition,
) (string, error) {
	var builder strings.Builder

	messages = append(messages, GenerateMessage{
		Role:    entity.MessageRoleUser,
		Content: contextSummaryPrompt,
	})

	err := generator.Generate(ctx, GenerateInput{
		Model:    model,
		Messages: messages,
		Tools:    tools,
	}, GenerateCallbacks{
		ContentDelta: func(delta string) {
			builder.WriteString(delta)
		},
	})
	if err != nil {
		return "", err
	}

	return strings.TrimSpace(builder.String()), nil
}
