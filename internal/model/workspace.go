package model

import "time"

type WorkspaceFileResponse struct {
	Name      string    `json:"name"`
	Path      string    `json:"path"`
	SizeBytes int64     `json:"size_bytes"`
	IsDir     bool      `json:"is_dir"`
	ModTime   time.Time `json:"mod_time"`
}

type ListWorkspaceFilesResponse struct {
	Items []WorkspaceFileResponse `json:"items"`
}
