package state

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestEnsureCreatesOnlyLDRDirectories(t *testing.T) {
	layout := NewLayout(t.TempDir())
	if err := layout.Ensure(); err != nil {
		t.Fatal(err)
	}
	for _, path := range []string{layout.Base, layout.Cache, layout.Profiles, layout.State} {
		info, err := os.Stat(path)
		if err != nil {
			t.Fatalf("stat %s: %v", path, err)
		}
		if !info.IsDir() {
			t.Fatalf("%s is not a directory", path)
		}
	}
}

func TestWriteJSONReplacesFileWithValidJSON(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "nested", "state.json")
	if err := WriteJSON(path, map[string]string{"status": "ready"}); err != nil {
		t.Fatal(err)
	}
	contents, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var decoded map[string]string
	if err := json.Unmarshal(contents, &decoded); err != nil {
		t.Fatalf("invalid JSON: %v", err)
	}
	if decoded["status"] != "ready" {
		t.Fatalf("decoded = %#v", decoded)
	}
	entries, err := os.ReadDir(filepath.Dir(path))
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 1 || entries[0].Name() != filepath.Base(path) {
		t.Fatalf("temporary files remain: %#v", entries)
	}
}
