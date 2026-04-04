package fs

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

// FsEvent types
const (
	EventCreated  = "created"
	EventModified = "modified"
	EventDeleted  = "deleted"
)

// FsEvent represents a filesystem change event.
type FsEvent struct {
	Path string `msgpack:"path" json:"path"`
	Type string `msgpack:"type" json:"type"`
}

// Watcher polls watched paths for filesystem changes.
type Watcher struct {
	root     string
	mu       sync.RWMutex
	watches  map[string]map[string]int64 // watched path -> {filepath -> modtime unix nano}
	callback func(FsEvent)
	done     chan struct{}
}

// NewWatcher creates a new polling-based file watcher.
func NewWatcher(root string, callback func(FsEvent)) *Watcher {
	return &Watcher{
		root:     root,
		watches:  make(map[string]map[string]int64),
		callback: callback,
		done:     make(chan struct{}),
	}
}

// resolvePath ensures the path is within the workspace root (same logic as Handler).
func (w *Watcher) resolvePath(path string) (string, error) {
	resolved := filepath.Join(w.root, filepath.Clean("/"+path))
	if !strings.HasPrefix(resolved, w.root) {
		return "", fmt.Errorf("path escapes workspace root: %s", path)
	}
	return resolved, nil
}

// Watch starts watching a path for changes. Directories are watched recursively.
func (w *Watcher) Watch(path string) error {
	resolved, err := w.resolvePath(path)
	if err != nil {
		return err
	}

	snapshot, err := w.snapshot(resolved)
	if err != nil {
		return fmt.Errorf("watch %s: %w", path, err)
	}

	w.mu.Lock()
	w.watches[resolved] = snapshot
	w.mu.Unlock()
	return nil
}

// Unwatch stops watching a path.
func (w *Watcher) Unwatch(path string) {
	resolved, _ := w.resolvePath(path)
	w.mu.Lock()
	delete(w.watches, resolved)
	w.mu.Unlock()
}

// Start begins the polling loop in a goroutine.
func (w *Watcher) Start() {
	go w.pollLoop()
}

// Stop stops the polling loop.
func (w *Watcher) Stop() {
	select {
	case <-w.done:
		// already stopped
	default:
		close(w.done)
	}
}

func (w *Watcher) pollLoop() {
	ticker := time.NewTicker(1 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-w.done:
			return
		case <-ticker.C:
			w.poll()
		}
	}
}

func (w *Watcher) poll() {
	w.mu.RLock()
	paths := make([]string, 0, len(w.watches))
	for p := range w.watches {
		paths = append(paths, p)
	}
	w.mu.RUnlock()

	for _, watchedPath := range paths {
		newSnapshot, err := w.snapshot(watchedPath)
		if err != nil {
			// If the watched path itself was deleted, emit a delete event
			w.mu.Lock()
			old := w.watches[watchedPath]
			if old != nil {
				for filePath := range old {
					w.callback(FsEvent{Path: w.relativePath(filePath), Type: EventDeleted})
				}
				w.watches[watchedPath] = make(map[string]int64)
			}
			w.mu.Unlock()
			continue
		}

		w.mu.Lock()
		oldSnapshot := w.watches[watchedPath]

		// Detect created and modified
		for filePath, newMod := range newSnapshot {
			oldMod, exists := oldSnapshot[filePath]
			if !exists {
				w.callback(FsEvent{Path: w.relativePath(filePath), Type: EventCreated})
			} else if newMod != oldMod {
				w.callback(FsEvent{Path: w.relativePath(filePath), Type: EventModified})
			}
		}

		// Detect deleted
		for filePath := range oldSnapshot {
			if _, exists := newSnapshot[filePath]; !exists {
				w.callback(FsEvent{Path: w.relativePath(filePath), Type: EventDeleted})
			}
		}

		w.watches[watchedPath] = newSnapshot
		w.mu.Unlock()
	}
}

// snapshot returns a map of file paths to their mod times for the given path.
// If path is a directory, it walks recursively.
func (w *Watcher) snapshot(path string) (map[string]int64, error) {
	info, err := os.Stat(path)
	if err != nil {
		return nil, err
	}

	result := make(map[string]int64)

	if !info.IsDir() {
		result[path] = info.ModTime().UnixNano()
		return result, nil
	}

	err = filepath.Walk(path, func(p string, fi os.FileInfo, err error) error {
		if err != nil {
			return nil // skip inaccessible entries
		}
		if !fi.IsDir() {
			result[p] = fi.ModTime().UnixNano()
		}
		return nil
	})
	return result, err
}

// relativePath converts an absolute path back to a path relative to root.
func (w *Watcher) relativePath(absPath string) string {
	rel, err := filepath.Rel(w.root, absPath)
	if err != nil {
		return absPath
	}
	return rel
}
