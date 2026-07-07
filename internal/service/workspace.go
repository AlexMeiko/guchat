package service

import (
	"context"
	"io"
	"os"

	"github.com/AlexMeiko/guchat/internal/sandbox"
)

type WorkspaceService struct {
	conversationService *ConversationService
	workspaceManager    *sandbox.WorkspaceManager
}

type SaveWorkspaceFileInput struct {
	ConversationID string
	FileName       string
	Reader         io.Reader
	Overwrite      bool
}

type ResolveWorkspaceFileResult struct {
	File *os.File
	Info sandbox.WorkspaceFile
}

func NewWorkspaceService(
	conversationService *ConversationService,
	workspaceManager *sandbox.WorkspaceManager,
) *WorkspaceService {
	return &WorkspaceService{
		conversationService: conversationService,
		workspaceManager:    workspaceManager,
	}
}

func (s *WorkspaceService) SaveFile(
	ctx context.Context,
	userID int64,
	input SaveWorkspaceFileInput,
) (*sandbox.WorkspaceFile, error) {
	if _, err := s.conversationService.Get(ctx, userID, input.ConversationID); err != nil {
		return nil, err
	}

	return s.workspaceManager.SaveFile(
		userID,
		input.ConversationID,
		input.FileName,
		input.Reader,
		input.Overwrite,
	)
}

func (s *WorkspaceService) ListFiles(
	ctx context.Context,
	userID int64,
	conversationID string,
) ([]sandbox.WorkspaceFile, error) {
	if _, err := s.conversationService.Get(ctx, userID, conversationID); err != nil {
		return nil, err
	}

	return s.workspaceManager.ListFiles(userID, conversationID)
}

func (s *WorkspaceService) DeleteItem(
	ctx context.Context,
	userID int64,
	conversationID string,
	name string,
) error {
	if _, err := s.conversationService.Get(ctx, userID, conversationID); err != nil {
		return err
	}

	return s.workspaceManager.DeleteItem(userID, conversationID, name)
}

func (s *WorkspaceService) ResolveFile(
	ctx context.Context,
	userID int64,
	conversationID string,
	name string,
) (*ResolveWorkspaceFileResult, error) {
	if _, err := s.conversationService.Get(ctx, userID, conversationID); err != nil {
		return nil, err
	}

	file, info, err := s.workspaceManager.OpenFile(userID, conversationID, name)
	if err != nil {
		return nil, err
	}

	return &ResolveWorkspaceFileResult{
		File: file,
		Info: info,
	}, nil
}
