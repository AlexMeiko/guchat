package service

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/AlexMeiko/guchat/internal/entity"
)

// newVisionAnalyzer 构造非视觉模型读图时使用的描述函数：构造阶段不查询，
// 只在被调用时取模型并发起生成，因此配置改动立刻生效、未读图的请求零开销。
// fallbackID <= 0 返回 nil，表示未配置 fallback。
func newVisionAnalyzer(models *ModelService, generators GeneratorFactory, fallbackID int64) AnalyzeImageFunc {
	if models == nil || generators == nil || fallbackID <= 0 {
		return nil
	}

	return func(ctx context.Context, mimeType string, data []byte, intent string) (string, error) {
		prompt := strings.TrimSpace(intent)
		if len(data) == 0 || prompt == "" {
			return "", errors.New("vision analyze requires image data and intent")
		}

		model, err := models.GetByID(ctx, fallbackID)
		if err != nil {
			return "", fmt.Errorf("vision fallback model %d: %w", fallbackID, err)
		}
		if !model.IsEnabled || !model.SupportsVision {
			return "", fmt.Errorf("vision fallback model %d is unavailable", fallbackID)
		}

		generator, err := generators.Get(model)
		if err != nil {
			return "", fmt.Errorf("vision fallback model %d: %w", fallbackID, err)
		}

		var builder strings.Builder

		err = generator.Generate(ctx, GenerateInput{
			Model: model,
			Messages: []GenerateMessage{{
				Role:    entity.MessageRoleUser,
				Content: prompt,
				Image:   &GenerateImage{MIMEType: mimeType, Bytes: data},
			}},
		}, GenerateCallbacks{
			ContentDelta: func(delta string) {
				builder.WriteString(delta)
			},
		})
		if err != nil {
			return "", fmt.Errorf("vision fallback model %d: %w", fallbackID, err)
		}

		return strings.TrimSpace(builder.String()), nil
	}
}
