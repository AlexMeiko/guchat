package service

import (
	"context"
	"fmt"
	"strings"

	"github.com/AlexMeiko/guchat/internal/entity"
)

const contextSummarySystemPrompt = `你是一个通用 AI Agent 的上下文压缩器。

你的任务是把“已有累积摘要”和“新增原始上下文”融合成一份新的完整累积摘要。
不要简单拼接。如果新增原始上下文与已有摘要冲突，以新增原始上下文为准。
除非旧信息本身对后续避免误用仍有价值，否则不要保留已失效的信息；如果有价值，把纠正后的有效约束或结论写入对应章节。

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

func buildContextSummaryMessages(previousSummary string, messages []GenerateMessage) []GenerateMessage {
	return []GenerateMessage{
		{
			Role:    entity.MessageRoleSystem,
			Content: contextSummarySystemPrompt,
		},
		{
			Role: entity.MessageRoleUser,
			Content: fmt.Sprintf(
				"已有累积摘要：\n\n%s\n\n需要融合的新增原始上下文：\n\n%s",
				summaryOrNone(previousSummary),
				formatGenerateMessagesForSummary(messages),
			),
		},
	}
}

func generateSummary(
	ctx context.Context,
	generator Generator,
	model *entity.ModelConfig,
	previousSummary string,
	messages []GenerateMessage,
) (string, error) {
	var builder strings.Builder

	err := generator.Generate(ctx, GenerateInput{
		Model:    model,
		Messages: buildContextSummaryMessages(previousSummary, messages),
		Tools:    nil,
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

func summaryOrNone(summary string) string {
	summary = strings.TrimSpace(summary)
	if summary == "" {
		return "(none)"
	}
	return summary
}

func formatGenerateMessagesForSummary(messages []GenerateMessage) string {
	var builder strings.Builder

	for _, message := range messages {
		builder.WriteString("### 消息\n")
		builder.WriteString("角色：")
		builder.WriteString(message.Role)
		builder.WriteString("\n")

		if content := strings.TrimSpace(message.Content); content != "" {
			builder.WriteString("内容：\n")
			builder.WriteString(content)
			builder.WriteString("\n")
		}

		if message.Role == entity.MessageRoleAssistant {
			if reasoningContent := strings.TrimSpace(message.ReasoningContent); reasoningContent != "" {
				builder.WriteString("推理内容：\n")
				builder.WriteString(reasoningContent)
				builder.WriteString("\n")
			}
		}

		for _, exchange := range message.ToolExchanges {
			builder.WriteString("工具调用：\n")
			builder.WriteString("名称：")
			builder.WriteString(exchange.Call.Name)
			builder.WriteString("\n")
			builder.WriteString("参数：\n")
			builder.WriteString(string(exchange.Call.Arguments))
			builder.WriteString("\n")
			builder.WriteString("结果：\n")
			builder.WriteString(string(exchange.Result.Result))
			builder.WriteString("\n")
		}

		builder.WriteString("\n")
	}

	raw := strings.TrimSpace(builder.String())
	if raw == "" {
		return "(none)"
	}
	return raw
}
