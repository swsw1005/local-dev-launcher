package discovery

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/swsw1005/local-dev-launcher/internal/state"
)

func TestDiscoverNormalizesSupportedProjectTasks(t *testing.T) {
	root := t.TempDir()
	writeFixture(t, root, "gradlew", "#!/bin/sh\n")
	writeFixture(t, root, "settings.gradle", "rootProject.name = 'sample'\n")
	writeFixture(t, root, "agent-api/build.gradle", "plugins { id 'org.springframework.boot' }\n")
	writeFixture(t, root, "frontend/package.json", `{"scripts":{"dev":"vite","test":"vitest"}}`)
	writeFixture(t, root, "service/pom.xml", "<project><artifactId>service</artifactId></project>")
	writeFixture(t, root, "tool/go.mod", "module example.com/tool\n\ngo 1.26\n")

	tasks, err := Discover(root)
	if err != nil {
		t.Fatal(err)
	}
	want := map[string]bool{
		"gradle.agent-api.bootRun": false,
		"node.frontend.dev":        false,
		"maven.service.package":    false,
		"go.tool.test":             false,
	}
	for _, task := range tasks {
		if _, ok := want[task.ID]; ok {
			want[task.ID] = true
		}
	}
	for id, found := range want {
		if !found {
			t.Errorf("missing %s in %#v", id, tasks)
		}
	}
}

func TestParseGradleTasksIncludesCustomAndModuleTasks(t *testing.T) {
	output := `
Application tasks
-----------------
homeops-agent-api:bootRun - Runs this project.

Verification tasks
------------------
homeops-agent-api:integrationTest - Runs integration tests.
localKindE2e - Runs local E2E checks.

BUILD SUCCESSFUL in 1s
`
	tasks := parseGradleTasks(output, "./gradlew")
	byID := map[string]bool{}
	for _, task := range tasks {
		byID[task.ID] = task.ID == "gradle.homeops-agent-api.integrationTest" && task.Args[0] == ":homeops-agent-api:integrationTest" && task.Group == "Verification tasks" ||
			byID[task.ID]
	}
	if !byID["gradle.homeops-agent-api.integrationTest"] {
		t.Fatalf("integrationTest was not parsed: %#v", tasks)
	}
	var local bool
	for _, task := range tasks {
		if task.ID == "gradle.root.localKindE2e" && task.Args[0] == "localKindE2e" {
			local = true
		}
	}
	if !local {
		t.Fatalf("root custom task was not parsed: %#v", tasks)
	}
}

func TestParseNodeScriptsUsesToolReportedJSON(t *testing.T) {
	scripts := parseNodeScripts([]byte(`{"scripts":{"dev":"vite","integration":"playwright test"}}`))
	if got, want := strings.Join(scripts, ","), "dev,integration"; got != want {
		t.Fatalf("scripts = %q, want %q", got, want)
	}
}

func TestParseMavenEffectivePOMIncludesPluginGoals(t *testing.T) {
	output := []byte(`Maven output
<?xml version="1.0"?>
<project><artifactId>service</artifactId><build><plugins><plugin><groupId>org.springframework.boot</groupId><artifactId>spring-boot-maven-plugin</artifactId><executions><execution><goals><goal>repackage</goal></goals></execution></executions></plugin></plugins></build></project>`)
	tasks, err := parseMavenEffectivePOM(output, "/workspace/example", ".", "./mvnw")
	if err != nil {
		t.Fatal(err)
	}
	if len(tasks) != 3 {
		t.Fatalf("tasks = %#v", tasks)
	}
	if got, want := tasks[0].Name, "spring-boot:repackage"; got != want {
		t.Fatalf("repackage name = %q, want %q", got, want)
	}
	if got, want := strings.Join(tasks[0].Args, " "), "org.springframework.boot:spring-boot-maven-plugin:repackage"; got != want {
		t.Fatalf("args = %q, want %q", got, want)
	}
	if got, want := tasks[1].Name, "spring-boot:run"; got != want || !tasks[1].Favorite {
		t.Fatalf("run task = %#v, want favorite %q", tasks[1], want)
	}
	if got, want := tasks[2].Name, "spring-boot:run (local)"; got != want || !tasks[2].Favorite || strings.Join(tasks[2].Args, " ") != "-Dspring-boot.run.profiles=local org.springframework.boot:spring-boot-maven-plugin:run" {
		t.Fatalf("local task = %#v", tasks[2])
	}
}

func TestLoadOrDiscoverUsesCacheUntilSourceChanges(t *testing.T) {
	root := t.TempDir()
	writeFixture(t, root, "package.json", `{"scripts":{"dev":"vite"}}`)
	layout := state.NewLayout(root)
	if err := layout.Ensure(); err != nil {
		t.Fatal(err)
	}
	first, err := LoadOrDiscover(root, layout, false)
	if err != nil {
		t.Fatal(err)
	}
	if first.Cached {
		t.Fatal("first discovery should not be cached")
	}
	second, err := LoadOrDiscover(root, layout, false)
	if err != nil {
		t.Fatal(err)
	}
	if !second.Cached {
		t.Fatal("unchanged project should use cache")
	}
	writeFixture(t, root, "package.json", `{"scripts":{"dev":"vite","test":"vitest"}}`)
	third, err := LoadOrDiscover(root, layout, false)
	if err != nil {
		t.Fatal(err)
	}
	if third.Cached || len(third.Tasks) != 2 {
		t.Fatalf("changed project should rediscover, got %#v", third)
	}
}

func TestDiscoverIgnoresGeneratedAndNestedWorktreeDirectories(t *testing.T) {
	root := t.TempDir()
	writeFixture(t, root, "package.json", `{"scripts":{"dev":"vite"}}`)
	writeFixture(t, root, ".worktrees/issue-1/package.json", `{"scripts":{"dev":"vite"}}`)
	writeFixture(t, root, "frontend/.next/standalone/package.json", `{"scripts":{"start":"next start"}}`)

	tasks, err := Discover(root)
	if err != nil {
		t.Fatal(err)
	}
	if len(tasks) != 1 || tasks[0].ID != "node.root.dev" {
		t.Fatalf("tasks = %#v", tasks)
	}
}

func TestDiscoverHonorsProjectIgnoreAndInvalidatesOnConfigChange(t *testing.T) {
	root := t.TempDir()
	writeFixture(t, root, "package.json", `{"scripts":{"dev":"vite"}}`)
	writeFixture(t, root, "vendor/package.json", `{"scripts":{"dev":"vite"}}`)
	writeFixture(t, root, ".ldr/project.toml", "version = 1\nignore = [\"vendor\"]\n")
	layout := state.NewLayout(root)
	if err := layout.Ensure(); err != nil {
		t.Fatal(err)
	}
	first, err := LoadOrDiscover(root, layout, false)
	if err != nil {
		t.Fatal(err)
	}
	if len(first.Tasks) != 1 || first.Tasks[0].ID != "node.root.dev" {
		t.Fatalf("tasks = %#v", first.Tasks)
	}
	second, err := LoadOrDiscover(root, layout, false)
	if err != nil || !second.Cached {
		t.Fatalf("second = %#v, err=%v", second, err)
	}
	if err := os.WriteFile(filepath.Join(root, ".ldr", "project.toml"), []byte("version = 1\nignore = []\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	third, err := LoadOrDiscover(root, layout, false)
	if err != nil {
		t.Fatal(err)
	}
	if third.Cached || len(third.Tasks) != 2 {
		t.Fatalf("changed config did not rediscover: %#v", third)
	}
}

func writeFixture(t *testing.T, root, relative, contents string) {
	t.Helper()
	path := filepath.Join(root, relative)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(contents), 0o644); err != nil {
		t.Fatal(err)
	}
}
