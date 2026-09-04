package handler

import (
	"errors"
	"mime"
	"net/http"

	"github.com/AlexMeiko/guchat/internal/model"
	"github.com/AlexMeiko/guchat/internal/sandbox"
	"github.com/AlexMeiko/guchat/internal/service"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

type WorkspaceHandler struct {
	workspaceService *service.WorkspaceService
	maxUploadBytes   int64
}

func NewWorkspaceHandler(workspaceService *service.WorkspaceService, maxUploadBytes int64) *WorkspaceHandler {
	return &WorkspaceHandler{
		workspaceService: workspaceService,
		maxUploadBytes:   maxUploadBytes,
	}
}

func (h *WorkspaceHandler) List(c *gin.Context) {
	conversationID := c.Param("conversation_id")
	if _, err := uuid.Parse(conversationID); err != nil {
		c.JSON(http.StatusBadRequest, model.ErrorResponse{Error: "invalid conversation id"})
		return
	}

	user, ok := requireCurrentUser(c)
	if !ok {
		return
	}

	files, err := h.workspaceService.ListFiles(c.Request.Context(), user.UserID, conversationID)
	if err != nil {
		if errors.Is(err, service.ErrConversationNotFound) {
			c.JSON(http.StatusNotFound, model.ErrorResponse{Error: "conversation not found"})
			return
		}

		c.JSON(http.StatusInternalServerError, model.ErrorResponse{Error: "internal server error"})
		return
	}

	items := make([]model.WorkspaceFileResponse, len(files))
	for i := range files {
		items[i] = newWorkspaceFileResponse(files[i])
	}

	c.JSON(http.StatusOK, model.ListWorkspaceFilesResponse{Items: items})
}

func (h *WorkspaceHandler) Upload(c *gin.Context) {
	conversationID := c.Param("conversation_id")
	if _, err := uuid.Parse(conversationID); err != nil {
		c.JSON(http.StatusBadRequest, model.ErrorResponse{Error: "invalid conversation id"})
		return
	}

	user, ok := requireCurrentUser(c)
	if !ok {
		return
	}

	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, h.maxUploadBytes+(1<<20))

	formFile, err := c.FormFile("file")
	if err != nil {
		var maxBytesErr *http.MaxBytesError
		if errors.As(err, &maxBytesErr) {
			c.JSON(http.StatusRequestEntityTooLarge, model.ErrorResponse{Error: "file too large"})
			return
		}
		c.JSON(http.StatusBadRequest, model.ErrorResponse{Error: "file is required"})
		return
	}

	if formFile.Size > h.maxUploadBytes {
		c.JSON(http.StatusRequestEntityTooLarge, model.ErrorResponse{Error: "file too large"})
		return
	}

	file, err := formFile.Open()
	if err != nil {
		c.JSON(http.StatusInternalServerError, model.ErrorResponse{Error: "internal server error"})
		return
	}
	defer file.Close()

	overwrite := c.PostForm("overwrite") == "true"

	saved, err := h.workspaceService.SaveFile(c.Request.Context(), user.UserID, service.SaveWorkspaceFileInput{
		ConversationID: conversationID,
		FileName:       formFile.Filename,
		Reader:         file,
		Overwrite:      overwrite,
	})
	if err != nil {
		switch {
		case errors.Is(err, service.ErrConversationNotFound):
			c.JSON(http.StatusNotFound, model.ErrorResponse{Error: "conversation not found"})
		case errors.Is(err, sandbox.ErrInvalidFileName):
			c.JSON(http.StatusBadRequest, model.ErrorResponse{Error: "invalid file name"})
		case errors.Is(err, sandbox.ErrFileExists):
			c.JSON(http.StatusConflict, model.ErrorResponse{Error: "file already exists"})
		case errors.Is(err, sandbox.ErrFileTooLarge):
			c.JSON(http.StatusRequestEntityTooLarge, model.ErrorResponse{Error: "file too large"})
		case errors.Is(err, sandbox.ErrWorkspaceItemNotRegular):
			c.JSON(http.StatusBadRequest, model.ErrorResponse{Error: "cannot overwrite non-regular file"})
		default:
			c.JSON(http.StatusInternalServerError, model.ErrorResponse{Error: "internal server error"})
		}
		return
	}

	c.JSON(http.StatusCreated, newWorkspaceFileResponse(*saved))
}

func (h *WorkspaceHandler) Delete(c *gin.Context) {
	conversationID := c.Param("conversation_id")
	if _, err := uuid.Parse(conversationID); err != nil {
		c.JSON(http.StatusBadRequest, model.ErrorResponse{Error: "invalid conversation id"})
		return
	}

	name := c.Query("name")
	if name == "" {
		c.JSON(http.StatusBadRequest, model.ErrorResponse{Error: "file name is required"})
		return
	}

	user, ok := requireCurrentUser(c)
	if !ok {
		return
	}

	err := h.workspaceService.DeleteItem(c.Request.Context(), user.UserID, conversationID, name)
	if err != nil {
		switch {
		case errors.Is(err, service.ErrConversationNotFound):
			c.JSON(http.StatusNotFound, model.ErrorResponse{Error: "conversation not found"})
		case errors.Is(err, sandbox.ErrInvalidFileName):
			c.JSON(http.StatusBadRequest, model.ErrorResponse{Error: "invalid file name"})
		case errors.Is(err, sandbox.ErrWorkspaceFileNotFound):
			c.JSON(http.StatusNotFound, model.ErrorResponse{Error: "file not found"})
		default:
			c.JSON(http.StatusInternalServerError, model.ErrorResponse{Error: "internal server error"})
		}
		return
	}

	c.Status(http.StatusNoContent)
}

func (h *WorkspaceHandler) Download(c *gin.Context) {
	conversationID := c.Param("conversation_id")
	if _, err := uuid.Parse(conversationID); err != nil {
		c.JSON(http.StatusBadRequest, model.ErrorResponse{Error: "invalid conversation id"})
		return
	}

	name := c.Query("name")
	if name == "" {
		c.JSON(http.StatusBadRequest, model.ErrorResponse{Error: "file name is required"})
		return
	}

	user, ok := requireCurrentUser(c)
	if !ok {
		return
	}

	result, err := h.workspaceService.ResolveFile(c.Request.Context(), user.UserID, conversationID, name)
	if err != nil {
		switch {
		case errors.Is(err, service.ErrConversationNotFound):
			c.JSON(http.StatusNotFound, model.ErrorResponse{Error: "conversation not found"})
		case errors.Is(err, sandbox.ErrInvalidFileName):
			c.JSON(http.StatusBadRequest, model.ErrorResponse{Error: "invalid file name"})
		case errors.Is(err, sandbox.ErrWorkspaceFileNotFound):
			c.JSON(http.StatusNotFound, model.ErrorResponse{Error: "file not found"})
		case errors.Is(err, sandbox.ErrWorkspaceItemIsDir):
			c.JSON(http.StatusBadRequest, model.ErrorResponse{Error: "cannot download directory"})
		case errors.Is(err, sandbox.ErrWorkspaceItemNotRegular):
			c.JSON(http.StatusBadRequest, model.ErrorResponse{Error: "cannot download non-regular file"})
		default:
			c.JSON(http.StatusInternalServerError, model.ErrorResponse{Error: "internal server error"})
		}
		return
	}

	defer result.File.Close()

	c.Header("Content-Disposition", mime.FormatMediaType("attachment", map[string]string{
		"filename": result.Info.Name,
	}))

	http.ServeContent(
		c.Writer,
		c.Request,
		result.Info.Name,
		result.Info.ModTime,
		result.File,
	)
}

func newWorkspaceFileResponse(file sandbox.WorkspaceFile) model.WorkspaceFileResponse {
	return model.WorkspaceFileResponse{
		Name:      file.Name,
		Path:      file.Path,
		SizeBytes: file.SizeBytes,
		IsDir:     file.IsDir,
		ModTime:   file.ModTime,
	}
}
