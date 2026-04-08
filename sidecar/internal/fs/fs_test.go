package fs

import (
	"os"
	"path/filepath"
	"testing"
)

func TestHandler_ReadFile(t *testing.T) {
	root := t.TempDir()
	h := NewHandler(root)

	content := []byte("hello world")
	if err := os.WriteFile(filepath.Join(root, "test.txt"), content, 0644); err != nil {
		t.Fatal(err)
	}

	got, err := h.ReadFile("test.txt")
	if err != nil {
		t.Fatalf("ReadFile() error: %v", err)
	}
	if string(got) != "hello world" {
		t.Errorf("content: expected %q, got %q", "hello world", string(got))
	}
}

func TestHandler_ReadFile_NotFound(t *testing.T) {
	root := t.TempDir()
	h := NewHandler(root)

	_, err := h.ReadFile("nonexistent.txt")
	if err == nil {
		t.Error("expected error for missing file")
	}
}

func TestHandler_WriteFile(t *testing.T) {
	root := t.TempDir()
	h := NewHandler(root)

	if err := h.WriteFile("output.txt", []byte("test data"), 0644); err != nil {
		t.Fatalf("WriteFile() error: %v", err)
	}

	got, err := os.ReadFile(filepath.Join(root, "output.txt"))
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != "test data" {
		t.Errorf("content: expected %q, got %q", "test data", string(got))
	}
}

func TestHandler_WriteFile_CreatesParentDirs(t *testing.T) {
	root := t.TempDir()
	h := NewHandler(root)

	if err := h.WriteFile("a/b/c/deep.txt", []byte("deep"), 0644); err != nil {
		t.Fatalf("WriteFile() error: %v", err)
	}

	got, err := os.ReadFile(filepath.Join(root, "a", "b", "c", "deep.txt"))
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != "deep" {
		t.Errorf("content: expected %q, got %q", "deep", string(got))
	}
}

func TestHandler_WriteFile_DefaultMode(t *testing.T) {
	root := t.TempDir()
	h := NewHandler(root)

	// mode=0 should default to 0644
	if err := h.WriteFile("default.txt", []byte("data"), 0); err != nil {
		t.Fatalf("WriteFile() error: %v", err)
	}

	_, err := os.Stat(filepath.Join(root, "default.txt"))
	if err != nil {
		t.Fatal(err)
	}
}

func TestHandler_ListDir(t *testing.T) {
	root := t.TempDir()
	h := NewHandler(root)

	os.WriteFile(filepath.Join(root, "file1.txt"), []byte("a"), 0644)
	os.WriteFile(filepath.Join(root, "file2.txt"), []byte("bb"), 0644)
	os.Mkdir(filepath.Join(root, "subdir"), 0755)

	entries, err := h.ListDir("")
	if err != nil {
		t.Fatalf("ListDir() error: %v", err)
	}
	if len(entries) != 3 {
		t.Fatalf("expected 3 entries, got %d", len(entries))
	}

	// Verify at least one dir entry exists
	hasDir := false
	for _, e := range entries {
		if e.IsDir && e.Name == "subdir" {
			hasDir = true
		}
	}
	if !hasDir {
		t.Error("expected to find 'subdir' directory entry")
	}
}

func TestHandler_ListDir_Empty(t *testing.T) {
	root := t.TempDir()
	h := NewHandler(root)

	entries, err := h.ListDir("")
	if err != nil {
		t.Fatalf("ListDir() error: %v", err)
	}
	if len(entries) != 0 {
		t.Errorf("expected 0 entries, got %d", len(entries))
	}
}

func TestHandler_Remove_File(t *testing.T) {
	root := t.TempDir()
	h := NewHandler(root)

	os.WriteFile(filepath.Join(root, "delete-me.txt"), []byte("bye"), 0644)

	if err := h.Remove("delete-me.txt", false); err != nil {
		t.Fatalf("Remove() error: %v", err)
	}

	if _, err := os.Stat(filepath.Join(root, "delete-me.txt")); !os.IsNotExist(err) {
		t.Error("file should not exist after removal")
	}
}

func TestHandler_Remove_DirRecursive(t *testing.T) {
	root := t.TempDir()
	h := NewHandler(root)

	dir := filepath.Join(root, "mydir")
	os.MkdirAll(filepath.Join(dir, "sub"), 0755)
	os.WriteFile(filepath.Join(dir, "sub", "file.txt"), []byte("x"), 0644)

	if err := h.Remove("mydir", true); err != nil {
		t.Fatalf("Remove(recursive) error: %v", err)
	}

	if _, err := os.Stat(dir); !os.IsNotExist(err) {
		t.Error("directory should not exist after recursive removal")
	}
}

func TestHandler_Remove_WorkspaceRoot(t *testing.T) {
	root := t.TempDir()
	h := NewHandler(root)

	err := h.Remove("", false)
	if err == nil {
		t.Error("expected error when removing workspace root")
	}
}

func TestHandler_Stat(t *testing.T) {
	root := t.TempDir()
	h := NewHandler(root)

	content := []byte("12345")
	os.WriteFile(filepath.Join(root, "stat.txt"), content, 0644)

	info, err := h.Stat("stat.txt")
	if err != nil {
		t.Fatalf("Stat() error: %v", err)
	}
	if info.Size() != 5 {
		t.Errorf("size: expected 5, got %d", info.Size())
	}
	if info.IsDir() {
		t.Error("expected file, not directory")
	}
}

func TestHandler_ResolvePath_Escapes(t *testing.T) {
	root := t.TempDir()
	h := NewHandler(root)

	escapePaths := []string{
		"../../etc/passwd",
		"../../../",
		"foo/../../..",
	}

	for _, p := range escapePaths {
		resolved, err := h.resolvePath(p)
		if err != nil {
			// Path rejected by validation -- correct behavior
			continue
		}
		// If no error, the resolved path must still be within root
		if !isWithinRoot(resolved, root) {
			t.Errorf("path %q resolved to %q which escapes root %q", p, resolved, root)
		}
	}
}

func TestHandler_NewHandler_DefaultRoot(t *testing.T) {
	h := NewHandler("")
	if h.root != workspaceRoot {
		t.Errorf("root: expected %q, got %q", workspaceRoot, h.root)
	}
}

// isWithinRoot checks if resolved path starts with root.
func isWithinRoot(resolved, root string) bool {
	rel, err := filepath.Rel(root, resolved)
	if err != nil {
		return false
	}
	return rel == "." || (len(rel) > 0 && rel[0] != '.')
}
