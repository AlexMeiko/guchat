package sandbox

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path"
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
var ErrInvalidImageStoreKey = errors.New("invalid image store key")

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

func parseFileName(name string) (string, error) {
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

func parseWorkspacePath(raw string) (string, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return "", ErrInvalidFileName
	}

	p := strings.ReplaceAll(raw, "\\", "/")

	if !strings.HasPrefix(p, "/workspace/") {
		return "", ErrInvalidFileName
	}
	p = strings.TrimPrefix(p, "/workspace/")
	if p == "" {
		return "", ErrInvalidFileName
	}

	cleaned := path.Clean("/" + p)
	if !strings.HasPrefix(cleaned, "/") || strings.HasPrefix(cleaned, "/..") {
		return "", ErrInvalidFileName
	}
	if cleaned == "/" {
		return "", ErrInvalidFileName
	}
	rel := strings.TrimPrefix(cleaned, "/")

	for _, part := range strings.Split(rel, "/") {
		if part == "" || part == "." || part == ".." {
			return "", ErrInvalidFileName
		}
		if strings.ContainsAny(part, `/\`) {
			return "", ErrInvalidFileName
		}
		for _, r := range part {
			if unicode.IsControl(r) {
				return "", ErrInvalidFileName
			}
		}
	}

	return rel, nil
}

func rejectSymlinkComponents(workspacePath, rel string) error {
	current := workspacePath
	for _, part := range strings.Split(rel, "/") {
		current = filepath.Join(current, part)
		info, err := os.Lstat(current)
		if err != nil {
			if os.IsNotExist(err) {
				return ErrWorkspaceFileNotFound
			}
			return err
		}
		if info.Mode()&os.ModeSymlink != 0 {
			return ErrWorkspaceItemNotRegular
		}
	}
	return nil
}

func (m *WorkspaceManager) resolveExistingPath(
	userID int64,
	conversationID string,
	raw string,
) (string, string, error) {
	rel, err := parseWorkspacePath(raw)
	if err != nil {
		return "", "", err
	}

	workspacePath, err := m.EnsureWorkspace(userID, conversationID)
	if err != nil {
		return "", "", err
	}

	if err := rejectSymlinkComponents(workspacePath, rel); err != nil {
		return "", "", err
	}

	return rel, filepath.Join(workspacePath, filepath.FromSlash(rel)), nil
}

func (m *WorkspaceManager) SaveFile(
	userID int64,
	conversationID string,
	originalName string,
	reader io.Reader,
	overwrite bool,
) (*WorkspaceFile, error) {
	name, err := parseFileName(originalName)
	if err != nil {
		return nil, err
	}

	rel, err := parseWorkspacePath("/workspace/" + name)
	if err != nil {
		return nil, err
	}

	workspacePath, err := m.EnsureWorkspace(userID, conversationID)
	if err != nil {
		return nil, err
	}

	targetPath := filepath.Join(workspacePath, filepath.FromSlash(rel))

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
		Name:      filepath.Base(rel),
		Path:      "/workspace/" + rel,
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
	_, targetPath, err := m.resolveExistingPath(userID, conversationID, name)
	if err != nil {
		return err
	}

	return os.RemoveAll(targetPath)
}

func (m *WorkspaceManager) OpenFile(
	userID int64,
	conversationID string,
	name string,
) (*os.File, WorkspaceFile, error) {
	rel, targetPath, err := m.resolveExistingPath(userID, conversationID, name)
	if err != nil {
		return nil, WorkspaceFile{}, err
	}

	file, info, err := openRegularNoFollow(targetPath)
	if err != nil {
		return nil, WorkspaceFile{}, err
	}

	return file, WorkspaceFile{
		Name:      filepath.Base(rel),
		Path:      "/workspace/" + rel,
		SizeBytes: info.Size(),
		IsDir:     false,
		ModTime:   info.ModTime(),
	}, nil
}
