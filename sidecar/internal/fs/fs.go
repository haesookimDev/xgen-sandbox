package fs

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
)

const workspaceRoot = "/home/sandbox/workspace"

// Handler provides sandboxed filesystem operations.
type Handler struct {
	root string
}

// NewHandler creates a new filesystem handler with the given root.
func NewHandler(root string) *Handler {
	if root == "" {
		root = workspaceRoot
	}
	return &Handler{root: root}
}

// resolvePath ensures the path is within the workspace root.
func (h *Handler) resolvePath(path string) (string, error) {
	resolved := filepath.Join(h.root, filepath.Clean("/"+path))
	if !strings.HasPrefix(resolved, h.root) {
		return "", fmt.Errorf("path escapes workspace root: %s", path)
	}
	return resolved, nil
}

// ReadFile reads the contents of a file.
func (h *Handler) ReadFile(path string) ([]byte, error) {
	resolved, err := h.resolvePath(path)
	if err != nil {
		return nil, err
	}
	return os.ReadFile(resolved)
}

// WriteFile writes content to a file, creating parent directories as needed.
func (h *Handler) WriteFile(path string, content []byte, mode os.FileMode) error {
	resolved, err := h.resolvePath(path)
	if err != nil {
		return err
	}
	if mode == 0 {
		mode = 0644
	}
	if err := os.MkdirAll(filepath.Dir(resolved), 0755); err != nil {
		return fmt.Errorf("create parent dirs: %w", err)
	}
	return os.WriteFile(resolved, content, mode)
}

// FileEntry represents a file or directory in a listing.
type FileEntry struct {
	Name    string `json:"name"`
	Size    int64  `json:"size"`
	IsDir   bool   `json:"is_dir"`
	ModTime int64  `json:"mod_time"`
}

// ListDir lists the contents of a directory.
func (h *Handler) ListDir(path string) ([]FileEntry, error) {
	resolved, err := h.resolvePath(path)
	if err != nil {
		return nil, err
	}

	entries, err := os.ReadDir(resolved)
	if err != nil {
		return nil, err
	}

	result := make([]FileEntry, 0, len(entries))
	for _, entry := range entries {
		info, err := entry.Info()
		if err != nil {
			continue
		}
		result = append(result, FileEntry{
			Name:    entry.Name(),
			Size:    info.Size(),
			IsDir:   entry.IsDir(),
			ModTime: info.ModTime().Unix(),
		})
	}
	return result, nil
}

// Remove removes a file or directory.
func (h *Handler) Remove(path string, recursive bool) error {
	resolved, err := h.resolvePath(path)
	if err != nil {
		return err
	}
	// Prevent removing the workspace root itself
	if resolved == h.root {
		return fmt.Errorf("cannot remove workspace root")
	}
	if recursive {
		return os.RemoveAll(resolved)
	}
	return os.Remove(resolved)
}

// Stat returns file info.
func (h *Handler) Stat(path string) (fs.FileInfo, error) {
	resolved, err := h.resolvePath(path)
	if err != nil {
		return nil, err
	}
	return os.Stat(resolved)
}
