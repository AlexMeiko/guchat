package sandbox

import (
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
var ErrWorkspaceFileNotFound = errors.New("workspace file not found")
var ErrWorkspaceItemIsDir = errors.New("workspace item is directory")

type WorkspaceFile struct {
	Name      string
	Path      string
	SizeBytes int64
	IsDir     bool
	ModTime   time.Time
}

type WorkspaceManager struct {
	dataRoot string
}

func NewWorkspaceManager(dataRoot string) (*WorkspaceManager, error) {
	dataRoot = strings.TrimSpace(dataRoot)
	if dataRoot == "" {
		return nil, fmt.Errorf("sandbox data root is required")
	}

	absRoot, err := filepath.Abs(dataRoot)
	if err != nil {
		return nil, err
	}

	return &WorkspaceManager{dataRoot: absRoot}, nil
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

	flags := os.O_WRONLY | os.O_CREATE
	if overwrite {
		flags |= os.O_TRUNC
	} else {
		flags |= os.O_EXCL
	}

	file, err := os.OpenFile(targetPath, flags, 0644)
	if err != nil {
		if errors.Is(err, os.ErrExist) {
			return nil, ErrFileExists
		}
		return nil, err
	}
	defer file.Close()

	if _, err := io.Copy(file, reader); err != nil {
		return nil, err
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

func (m *WorkspaceManager) ResolveFilePath(
	userID int64,
	conversationID string,
	name string,
) (string, WorkspaceFile, error) {
	name, err := cleanFileName(name)
	if err != nil {
		return "", WorkspaceFile{}, err
	}

	workspacePath, err := m.EnsureWorkspace(userID, conversationID)
	if err != nil {
		return "", WorkspaceFile{}, err
	}

	targetPath := filepath.Join(workspacePath, name)

	info, err := os.Stat(targetPath)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return "", WorkspaceFile{}, ErrWorkspaceFileNotFound
		}
		return "", WorkspaceFile{}, err
	}

	if info.IsDir() {
		return "", WorkspaceFile{}, ErrWorkspaceItemIsDir
	}

	return targetPath, WorkspaceFile{
		Name:      name,
		Path:      "/workspace/" + name,
		SizeBytes: info.Size(),
		IsDir:     false,
		ModTime:   info.ModTime(),
	}, nil
}
