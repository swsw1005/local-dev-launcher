package runtimes

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/swsw1005/local-dev-launcher/internal/domain"
)

func TestResolveNodeTaskUsesAbsoluteRuntimeCommand(t *testing.T) {
	root := t.TempDir()
	store := filepath.Join(root, "runtimes")
	for _, major := range []string{"20", "24"} {
		bin := filepath.Join(store, "node", major, "bin")
		if err := os.MkdirAll(bin, 0o755); err != nil {
			t.Fatal(err)
		}
		for _, name := range []string{"node", "npm"} {
			if err := os.WriteFile(filepath.Join(bin, name), []byte("#!/bin/sh\n"), 0o755); err != nil {
				t.Fatal(err)
			}
		}
	}
	if err := os.WriteFile(filepath.Join(root, "package.json"), []byte(`{"engines":{"node":">=20 <25"}}`), 0o644); err != nil {
		t.Fatal(err)
	}
	t.Setenv(overrideKey, store)

	resolution, err := ResolveTask(root, domain.Task{Adapter: "node", Command: "npm", Args: []string{"run", "build"}})
	if err != nil {
		t.Fatal(err)
	}
	want := filepath.Join(store, "node", "20", "bin", "npm")
	if resolution.Task.Command != want {
		t.Fatalf("command = %q, want %q", resolution.Task.Command, want)
	}
	if !strings.HasPrefix(resolution.Environment["PATH"], filepath.Join(store, "node", "20", "bin")+string(os.PathListSeparator)) {
		t.Fatalf("PATH = %q, does not start with selected Node bin", resolution.Environment["PATH"])
	}
}

func TestResolveNodeMajorDefaultsToNode24(t *testing.T) {
	root := t.TempDir()
	for _, major := range []string{"20", "24", "26"} {
		bin := filepath.Join(root, major, "bin")
		if err := os.MkdirAll(bin, 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(bin, "node"), []byte("node"), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	major, err := resolveNodeMajor(root, "")
	if err != nil {
		t.Fatal(err)
	}
	if major != 24 {
		t.Fatalf("major = %d, want 24", major)
	}
}
