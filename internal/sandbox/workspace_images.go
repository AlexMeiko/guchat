package sandbox

import (
	"errors"
	"os"
	"path/filepath"
	"strconv"
)

func validateImageStoreKey(storeKey string) error {
	if len(storeKey) != 64 {
		return ErrInvalidImageStoreKey
	}
	for _, c := range storeKey {
		if c >= '0' && c <= '9' || c >= 'a' && c <= 'f' {
			continue
		}
		return ErrInvalidImageStoreKey
	}
	return nil
}

func (m *WorkspaceManager) ImagesPath(userID int64, conversationID string) string {
	return filepath.Join(
		m.dataRoot,
		strconv.FormatInt(userID, 10),
		conversationID,
		"images",
	)
}

func (m *WorkspaceManager) WriteStoredImage(userID int64, conversationID, storeKey string, data []byte) error {
	if err := validateImageStoreKey(storeKey); err != nil {
		return err
	}

	imagesPath := m.ImagesPath(userID, conversationID)
	if err := os.MkdirAll(imagesPath, 0755); err != nil {
		return err
	}

	targetPath := filepath.Join(imagesPath, storeKey)
	file, err := createExclusiveNoFollow(targetPath, 0644)
	if err != nil {
		if errors.Is(err, os.ErrExist) {
			return nil
		}
		return err
	}

	_, writeErr := file.Write(data)
	closeErr := file.Close()
	if writeErr != nil {
		_ = os.Remove(targetPath)
		return writeErr
	}
	if closeErr != nil {
		_ = os.Remove(targetPath)
		return closeErr
	}
	return nil
}

func (m *WorkspaceManager) OpenStoredImage(userID int64, conversationID, storeKey string) (*os.File, error) {
	if err := validateImageStoreKey(storeKey); err != nil {
		return nil, err
	}

	file, _, err := openRegularNoFollow(filepath.Join(m.ImagesPath(userID, conversationID), storeKey))
	return file, err
}
