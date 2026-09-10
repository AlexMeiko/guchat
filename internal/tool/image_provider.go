package tool

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"strings"

	"github.com/AlexMeiko/guchat/internal/imageproc"
	"github.com/AlexMeiko/guchat/internal/sandbox"
	"github.com/AlexMeiko/guchat/internal/service"
)

const ToolReadImage = "read_image"

type ReadImageResult struct {
	OK             bool   `json:"ok"`
	Error          string `json:"error,omitempty"`
	Path           string `json:"path,omitempty"`
	MIMEType       string `json:"mime_type,omitempty"`
	Width          int    `json:"width,omitempty"`
	Height         int    `json:"height,omitempty"`
	SizeBytes      int64  `json:"size_bytes,omitempty"`
	StoreKey       string `json:"store_key,omitempty"`
	StoredMIMEType string `json:"stored_mime_type,omitempty"`
	StoredWidth    int    `json:"stored_width,omitempty"`
	StoredHeight   int    `json:"stored_height,omitempty"`
	Intent         string `json:"intent,omitempty"`
	Analysis       string `json:"analysis,omitempty"`
}

type ImageProvider struct {
	workspace *sandbox.WorkspaceManager
	maxBytes  int64
	maxEdge   int
}

type readImageArgs struct {
	Path   string `json:"path"`
	Intent string `json:"intent"`
}

func NewImageProvider(workspace *sandbox.WorkspaceManager, maxBytes int64, maxEdge int) *ImageProvider {
	return &ImageProvider{
		workspace: workspace,
		maxBytes:  maxBytes,
		maxEdge:   maxEdge,
	}
}

func (p *ImageProvider) Name() string {
	return "image"
}

func (p *ImageProvider) ListTools(ctx context.Context, user service.UserContext) ([]service.ToolDefinition, error) {
	return []service.ToolDefinition{
		{
			Name:        ToolReadImage,
			Description: "读取当前会话工作区中的一张图片。path 必须是 /workspace/ 开头的绝对路径。支持 JPEG、PNG、WebP。每次调用只能读一张图。",
			Parameters: json.RawMessage(`{
                "type": "object",
                "properties": {
                    "path": {
                        "type": "string",
                        "description": "工作区绝对路径，必须是 /workspace/ 开头。支持 JPEG、PNG、WebP。"
                    },
                    "intent": {
                        "type": "string",
                        "description": "非视觉模型描述图片时想关注的内容，可选。"
                    }
                },
                "required": ["path"],
                "additionalProperties": false
            }`),
		},
	}, nil
}

func (p *ImageProvider) CallTool(ctx context.Context, user service.UserContext, name string, args json.RawMessage) (service.ToolResult, error) {
	if name != ToolReadImage {
		return service.ToolResult{}, service.ErrToolNotFound
	}
	return p.readImage(ctx, user, args)
}

func (p *ImageProvider) readImage(ctx context.Context, user service.UserContext, args json.RawMessage) (service.ToolResult, error) {
	if user.UserID <= 0 || strings.TrimSpace(user.ConversationID) == "" {
		return service.ToolResult{}, errors.New("read_image requires user and conversation id")
	}

	var input readImageArgs
	if err := json.Unmarshal(args, &input); err != nil {
		return service.ToolResult{}, err
	}
	input.Path = strings.TrimSpace(input.Path)
	input.Intent = strings.TrimSpace(input.Intent)
	if input.Path == "" {
		return marshalReadImageResult(ReadImageResult{
			OK:    false,
			Error: "path is required",
		})
	}

	file, info, err := p.workspace.OpenFile(user.UserID, user.ConversationID, input.Path)
	if err != nil {
		return marshalReadImageError(err)
	}

	data, readErr := io.ReadAll(io.LimitReader(file, p.maxBytes+1))
	closeErr := file.Close()
	if readErr != nil {
		return marshalReadImageError(readErr)
	}
	if closeErr != nil {
		return marshalReadImageError(closeErr)
	}
	if int64(len(data)) > p.maxBytes {
		return marshalReadImageResult(ReadImageResult{
			OK:    false,
			Error: "image exceeds size limit",
		})
	}

	processed, err := imageproc.Process(data, imageproc.Options{MaxEdge: p.maxEdge})
	if err != nil {
		return marshalReadImageError(err)
	}

	if err := p.workspace.WriteStoredImage(user.UserID, user.ConversationID, processed.StoreKey, processed.Bytes); err != nil {
		return marshalReadImageError(err)
	}

	result := ReadImageResult{
		OK:             true,
		Path:           info.Path,
		MIMEType:       processed.OriginalMIME,
		Width:          processed.OriginalWidth,
		Height:         processed.OriginalHeight,
		SizeBytes:      processed.OriginalSize,
		StoreKey:       processed.StoreKey,
		StoredMIMEType: processed.MIME,
		StoredWidth:    processed.Width,
		StoredHeight:   processed.Height,
		Intent:         input.Intent,
	}
	return marshalReadImageResult(result)
}

func marshalReadImageError(err error) (service.ToolResult, error) {
	msg := err.Error()
	switch {
	case errors.Is(err, sandbox.ErrInvalidFileName):
		msg = "invalid file path"
	case errors.Is(err, sandbox.ErrWorkspaceFileNotFound):
		msg = "file not found"
	case errors.Is(err, sandbox.ErrWorkspaceItemIsDir):
		msg = "path is a directory"
	case errors.Is(err, sandbox.ErrWorkspaceItemNotRegular):
		msg = "path is not a regular file"
	case errors.Is(err, imageproc.ErrUnsupportedFormat):
		msg = "unsupported image format"
	}
	return marshalReadImageResult(ReadImageResult{
		OK:    false,
		Error: msg,
	})
}

func marshalReadImageResult(value ReadImageResult) (service.ToolResult, error) {
	payload, err := json.Marshal(value)
	if err != nil {
		return service.ToolResult{}, err
	}
	return service.ToolResult{
		Name:   ToolReadImage,
		Result: payload,
	}, nil
}
