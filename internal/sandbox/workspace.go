package sandbox

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
	"unicode"
)

var ErrInvalidFileName = errors.New("invalid file name")
var ErrFileExists = errors.New("file already exists")
var ErrFileTooLarge = errors.New("file too large")
var ErrWorkspaceFileNotFound = errors.New("workspace file not found")
var ErrWorkspaceItemIsDir = errors.New("workspace item is directory")
var ErrWorkspaceItemNotRegular = errors.New("workspace item is not regular file")

type WorkspaceFile struct {
	Name      string
	Path      string
	SizeBytes int64
	IsDir     bool
	ModTime   time.Time
}

type WorkspaceManager struct {
	dataRoot       string
	maxUploadBytes int64
}

func NewWorkspaceManager(dataRoot string, maxUploadBytes int64) (*WorkspaceManager, error) {
	dataRoot = strings.TrimSpace(dataRoot)
	if dataRoot == "" {
		return nil, fmt.Errorf("sandbox data root is required")
	}

	absRoot, err := filepath.Abs(dataRoot)
	if err != nil {
		return nil, err
	}

	return &WorkspaceManager{dataRoot: absRoot, maxUploadBytes: maxUploadBytes}, nil
}

func (m *WorkspaceManager) WorkspacePath(userID int64, conversationID string) string {
	return filepath.Join(
		m.dataRoot,
		strconv.FormatInt(userID, 10),
		conversationID,
		"workspace",
	)
}

func (m *WorkspaceManager) EnsureWorkspace(userID int64, conversationID string) (string, error) {
	path := m.WorkspacePath(userID, conversationID)
	if err := os.MkdirAll(path, 0755); err != nil {
		return "", err
	}
	return path, nil
}

func (m *WorkspaceManager) DeleteConversation(ctx context.Context, userID int64, conversationID string) error {
	if err := ctx.Err(); err != nil {
		return err
	}

	path := filepath.Join(
		m.dataRoot,
		strconv.FormatInt(userID, 10),
		conversationID,
	)
	return os.RemoveAll(path)
}

func cleanFileName(name string) (string, error) {
	name = strings.TrimSpace(name)
	if name == "" {
		return "", ErrInvalidFileName
	}

	base := filepath.Base(name)
	if base != name || base == "." || base == ".." {
		return "", ErrInvalidFileName
	}

	if strings.ContainsAny(base, `/\`) {
		return "", ErrInvalidFileName
	}

	for _, r := range base {
		if unicode.IsControl(r) {
			return "", ErrInvalidFileName
		}
	}

	return base, nil
}

func (m *WorkspaceManager) SaveFile(
	userID int64,
	conversationID string,
	originalName string,
	reader io.Reader,
	overwrite bool,
) (*WorkspaceFile, error) {
	name, err := cleanFileName(originalName)
	if err != nil {
		return nil, err
	}

	workspacePath, err := m.EnsureWorkspace(userID, conversationID)
	if err != nil {
		return nil, err
	}

	targetPath := filepath.Join(workspacePath, name)

	var file *os.File
	if overwrite {
		file, err = openRegularForWriteNoFollow(targetPath, 0644)
	} else {
		file, err = os.OpenFile(targetPath, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0644)
	}
	if err != nil {
		if errors.Is(err, os.ErrExist) {
			return nil, ErrFileExists
		}
		return nil, err
	}
	defer file.Close()

	written, err := io.Copy(file, io.LimitReader(reader, m.maxUploadBytes+1))
	if err != nil {
		return nil, err
	}
	if written > m.maxUploadBytes {
		_ = file.Close()
		_ = os.Remove(targetPath)
		return nil, ErrFileTooLarge
	}

	info, err := file.Stat()
	if err != nil {
		return nil, err
	}

	return &WorkspaceFile{
		Name:      name,
		Path:      "/workspace/" + name,
		SizeBytes: info.Size(),
		IsDir:     false,
		ModTime:   info.ModTime(),
	}, nil
}

func (m *WorkspaceManager) ListFiles(userID int64, conversationID string) ([]WorkspaceFile, error) {
	workspacePath, err := m.EnsureWorkspace(userID, conversationID)
	if err != nil {
		return nil, err
	}

	entries, err := os.ReadDir(workspacePath)
	if err != nil {
		return nil, err
	}

	files := make([]WorkspaceFile, 0, len(entries))
	for _, entry := range entries {
		info, err := entry.Info()
		if err != nil {
			return nil, err
		}

		files = append(files, WorkspaceFile{
			Name:      entry.Name(),
			Path:      "/workspace/" + entry.Name(),
			SizeBytes: info.Size(),
			IsDir:     entry.IsDir(),
			ModTime:   info.ModTime(),
		})
	}

	return files, nil
}

func (m *WorkspaceManager) DeleteItem(userID int64, conversationID string, name string) error {
	name, err := cleanFileName(name)
	if err != nil {
		return err
	}

	workspacePath, err := m.EnsureWorkspace(userID, conversationID)
	if err != nil {
		return err
	}

	targetPath := filepath.Join(workspacePath, name)

	if _, err := os.Stat(targetPath); err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return ErrWorkspaceFileNotFound
		}
		return err
	}

	return os.RemoveAll(targetPath)
}

func (m *WorkspaceManager) OpenFile(
	userID int64,
	conversationID string,
	name string,
) (*os.File, WorkspaceFile, error) {
	name, err := cleanFileName(name)
	if err != nil {
		return nil, WorkspaceFile{}, err
	}

	workspacePath, err := m.EnsureWorkspace(userID, conversationID)
	if err != nil {
		return nil, WorkspaceFile{}, err
	}

	targetPath := filepath.Join(workspacePath, name)

	file, info, err := openRegularNoFollow(targetPath)
	if err != nil {
		return nil, WorkspaceFile{}, err
	}

	return file, WorkspaceFile{
		Name:      name,
		Path:      "/workspace/" + name,
		SizeBytes: info.Size(),
		IsDir:     false,
		ModTime:   info.ModTime(),
	}, nil
}
