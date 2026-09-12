package project

import (
	"os"
	"path/filepath"
	"testing"
)

func TestFindRootUsesClosestProjectMarker(t *testing.T) {
	temp := t.TempDir()
	root := filepath.Join(temp, "project")
	nested := filepath.Join(root, "module", "src")
	if err := os.MkdirAll(nested, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "package.json"), []byte("{}"), 0o644); err != nil {
		t.Fatal(err)
	}

	got, err := FindRoot(nested)
	if err != nil {
		t.Fatal(err)
	}
	if got != root {
		t.Fatalf("root = %q, want %q", got, root)
	}
}

func TestFindRootFallsBackToStart(t *testing.T) {
	start := t.TempDir()
	got, err := FindRoot(start)
	if err != nil {
		t.Fatal(err)
	}
	if got != start {
		t.Fatalf("root = %q, want %q", got, start)
	}
}
