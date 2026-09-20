package project

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadConfig(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, ".ldr")
	if err := os.MkdirAll(path, 0o755); err != nil {
		t.Fatal(err)
	}
	contents := "version = 1\ndefault_profile = \"backend-local\"\nignore = [\"vendor\", \"generated\"]\n\n[runtimes]\njava = \"21\"\nnode = \"24\"\n"
	if err := os.WriteFile(filepath.Join(path, "project.toml"), []byte(contents), 0o644); err != nil {
		t.Fatal(err)
	}
	config, err := LoadConfig(root)
	if err != nil {
		t.Fatal(err)
	}
	if config.DefaultProfile != "backend-local" || len(config.Ignore) != 2 || config.Runtimes["java"] != "21" {
		t.Fatalf("config = %#v", config)
	}
}
