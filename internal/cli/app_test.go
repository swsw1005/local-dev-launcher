package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/swsw1005/local-dev-launcher/internal/gitignore"
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

	if err := app.Run(context.Background(), []string{"init", "--yes"}, root); err != nil {
		t.Fatal(err)
	}
	for _, relative := range []string{".ldr/cache", ".ldr/profiles", ".ldr/state"} {
		if info, err := os.Stat(filepath.Join(root, relative)); err != nil || !info.IsDir() {
			t.Fatalf("missing %s: %v", relative, err)
		}
	}
	if !strings.Contains(errOut.String(), "WARNING: .ldr/ is not ignored") || !strings.Contains(errOut.String(), "경고: .ldr/ 디렉터리가 Git에서 무시되지 않습니다") {
		t.Fatalf("warning = %q", errOut.String())
	}
}

func TestNoArgumentsInitializesWithoutBlocking(t *testing.T) {
	root := t.TempDir()
	var out, errOut bytes.Buffer
	app := New(strings.NewReader(""), &out, &errOut)
	if err := app.Run(context.Background(), nil, root); err != nil {
		t.Fatal(err)
	}
	if info, err := os.Stat(filepath.Join(root, ".ldr", "cache")); err != nil || !info.IsDir() {
		t.Fatalf(".ldr/cache missing: %v", err)
	}
}

func TestExistingStateDoesNotPrintAStatusLine(t *testing.T) {
	root := t.TempDir()
	var out, errOut bytes.Buffer
	app := App{out: &out, errOut: &errOut, git: stubGit{}, findRoot: func(string) (string, error) { return root, nil }}

	if err := app.Run(context.Background(), nil, root); err != nil {
		t.Fatal(err)
	}
	out.Reset()
	if err := app.Run(context.Background(), nil, root); err != nil {
		t.Fatal(err)
	}
	if out.Len() != 0 {
		t.Fatalf("output = %q, want no state status", out.String())
	}
}

func TestHelpAndVersion(t *testing.T) {
	var out, errOut bytes.Buffer
	app := New(strings.NewReader(""), &out, &errOut)
	if err := app.Run(context.Background(), []string{"--help"}, t.TempDir()); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), "Usage:") {
		t.Fatalf("help = %q", out.String())
	}
	out.Reset()
	if err := app.Run(context.Background(), []string{"-v"}, t.TempDir()); err != nil {
		t.Fatal(err)
	}
	if got, want := out.String(), "ldr "+Version+"\n"; got != want {
		t.Fatalf("version = %q, want %q", got, want)
	}
}

func TestParseInstallArgs(t *testing.T) {
	request, choose, err := parseInstallArgs([]string{"java", "21"})
	if err != nil || choose || request.Runtime != "java" || request.Version != "21" {
		t.Fatalf("java request = %#v, choose=%v, err=%v", request, choose, err)
	}
	request, choose, err = parseInstallArgs([]string{"node", "--lts"})
	if err != nil || choose || !request.LTS || request.Runtime != "node" {
		t.Fatalf("node LTS request = %#v, choose=%v, err=%v", request, choose, err)
	}
	_, choose, err = parseInstallArgs([]string{"go"})
	if err != nil || !choose {
		t.Fatalf("Go without a family should prompt: choose=%v, err=%v", choose, err)
	}
	request, choose, err = parseInstallArgs([]string{"go", "latest"})
	if err != nil || choose || request.Version != "" || request.Runtime != "go" {
		t.Fatalf("Go latest request = %#v, choose=%v, err=%v", request, choose, err)
	}
	request, choose, err = parseInstallArgs([]string{"go", "1.26.3"})
	if err != nil || choose || request.Version != "1.26.3" {
		t.Fatalf("Go exact request = %#v, choose=%v, err=%v", request, choose, err)
	}
}

func TestRuntimeHelp(t *testing.T) {
	var out, errOut bytes.Buffer
	app := New(strings.NewReader(""), &out, &errOut)
	if err := app.Run(context.Background(), []string{"runtime", "--help"}, t.TempDir()); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), "runtime use") {
		t.Fatalf("help = %q", out.String())
	}
	if !strings.Contains(out.String(), "runtime search") {
		t.Fatalf("runtime search help = %q", out.String())
	}
}

func TestInitShellHelp(t *testing.T) {
	var out, errOut bytes.Buffer
	app := New(strings.NewReader(""), &out, &errOut)
	if err := app.Run(context.Background(), []string{"init-shell", "--help"}, t.TempDir()); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), "shared shell paths") {
		t.Fatalf("help = %q", out.String())
	}
}

func TestListOutputsDiscoveredTasksAsJSON(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "package.json"), []byte(`{"scripts":{"dev":"vite"}}`), 0o644); err != nil {
		t.Fatal(err)
	}
	var out, errOut bytes.Buffer
	app := App{out: &out, errOut: &errOut, git: stubGit{}, findRoot: func(string) (string, error) { return root, nil }, version: Version}
	if err := app.Run(context.Background(), []string{"list", "--json"}, root); err != nil {
		t.Fatal(err)
	}
	var tasks []struct {
		ID string `json:"id"`
	}
	if err := json.Unmarshal(out.Bytes(), &tasks); err != nil {
		t.Fatal(err)
	}
	if len(tasks) != 1 || tasks[0].ID != "node.root.dev" {
		t.Fatalf("tasks = %#v", tasks)
	}
}

func TestRunReportsMissingTask(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "package.json"), []byte(`{"scripts":{"dev":"vite"}}`), 0o644); err != nil {
		t.Fatal(err)
	}
	var out, errOut bytes.Buffer
	app := App{out: &out, errOut: &errOut, git: stubGit{}, findRoot: func(string) (string, error) { return root, nil }, version: Version}
	err := app.Run(context.Background(), []string{"run", "node.root.missing"}, root)
	if err == nil || !strings.Contains(err.Error(), "was not found") {
		t.Fatalf("error = %v", err)
	}
}

func TestProfileCloneAndList(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "package.json"), []byte(`{"scripts":{"dev":"vite"}}`), 0o644); err != nil {
		t.Fatal(err)
	}
	var out, errOut bytes.Buffer
	app := App{out: &out, errOut: &errOut, git: stubGit{}, findRoot: func(string) (string, error) { return root, nil }, version: Version}
	if err := app.Run(context.Background(), []string{"profile", "clone", "node.root.dev", "frontend-local"}, root); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), "Created profile") {
		t.Fatalf("output = %q", out.String())
	}
	out.Reset()
	if err := app.Run(context.Background(), []string{"profile", "list"}, root); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), "frontend-local\tnode.root.dev\tREADY") {
		t.Fatalf("output = %q", out.String())
	}
}
