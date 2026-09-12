package cli

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/swim/local-dev-runner/internal/gitignore"
)

type stubGit struct{ status gitignore.Status }

func (s stubGit) InsideWorkTree(context.Context, string) (bool, error) {
	return s.status.InRepository, nil
}
func (s stubGit) IsIgnored(context.Context, string, string) (bool, error) {
	return s.status.Ignored, nil
}

func TestInitCreatesLayoutAndWarnsForUnignoredGitState(t *testing.T) {
	root := t.TempDir()
	var out, errOut bytes.Buffer
	app := App{out: &out, errOut: &errOut, git: stubGit{status: gitignore.Status{InRepository: true}}, findRoot: func(string) (string, error) { return root, nil }}

	if err := app.Run(context.Background(), []string{"init"}, root); err != nil {
		t.Fatal(err)
	}
	for _, relative := range []string{".ldr/cache", ".ldr/profiles", ".ldr/state"} {
		if info, err := os.Stat(filepath.Join(root, relative)); err != nil || !info.IsDir() {
			t.Fatalf("missing %s: %v", relative, err)
		}
	}
	if !strings.Contains(errOut.String(), "WARNING: .ldr/ is not ignored") {
		t.Fatalf("warning = %q", errOut.String())
	}
}

func TestNoArgumentsDoesNotInitialize(t *testing.T) {
	root := t.TempDir()
	var out, errOut bytes.Buffer
	app := New(&out, &errOut)
	if err := app.Run(context.Background(), nil, root); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(root, ".ldr")); !os.IsNotExist(err) {
		t.Fatalf(".ldr should not exist, stat error = %v", err)
	}
}
