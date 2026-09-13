package discovery

import (
	"os"
	"path/filepath"
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
