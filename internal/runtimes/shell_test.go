package runtimes

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestInitShellPreservesExistingPathsAndAddsShellHooks(t *testing.T) {
	home := t.TempDir()
	store := filepath.Join(home, "Library", "Application Support", "local-dev-runner", "runtimes")
	t.Setenv("HOME", home)
	t.Setenv("LDR_RUNTIME_HOME", store)
	existing := "export PATH=\"$HOME/custom/bin:$PATH\"\n"
	if err := os.WriteFile(filepath.Join(home, ".shell_paths"), []byte(existing), 0o644); err != nil {
		t.Fatal(err)
	}
	result, err := InitShell()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := InitShell(); err != nil {
		t.Fatalf("second init-shell run failed: %v", err)
	}
	contents, err := os.ReadFile(result.ShellPaths)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(contents), existing) || strings.Count(string(contents), shellManagedStart) != 1 || !strings.Contains(string(contents), "banner.sh") {
		t.Fatalf("shell paths = %q", contents)
	}
	first := append([]byte(nil), contents...)
	if _, err := InitShell(); err != nil {
		t.Fatalf("third init-shell run failed: %v", err)
	}
	contents, err = os.ReadFile(result.ShellPaths)
	if err != nil || string(contents) != string(first) {
		t.Fatalf("shell paths changed on repeated init: %q, err=%v", contents, err)
	}
	for _, name := range []string{".bashrc", ".zshrc"} {
		contents, err := os.ReadFile(filepath.Join(home, name))
		if err != nil || strings.Count(string(contents), shellSourceLine) != 1 {
			t.Fatalf("%s = %q, err=%v", name, contents, err)
		}
	}
	if info, err := os.Stat(result.ShellHome); err != nil || !info.IsDir() {
		t.Fatalf("shell home = %q, err=%v", result.ShellHome, err)
	}
}
