package discovery

import (
	"context"
	"encoding/json"
	"encoding/xml"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"

	"github.com/swsw1005/local-dev-launcher/internal/domain"
	"github.com/swsw1005/local-dev-launcher/internal/runtimes"
)

func discoverGradleTaskReport(root string, _ []domain.Task) ([]domain.Task, error) {
	return discoverGradleTasks(root)
}

// discoverNodeTaskReport asks npm or pnpm for the scripts it can run. The
// package.json parser remains the fallback because it is already authoritative
// when the Node runtime has not been installed yet.
func discoverNodeTaskReport(root string, tasks []domain.Task) ([]domain.Task, error) {
	byModule := map[string]domain.Task{}
	for _, task := range tasks {
		if task.Adapter == "node" {
			byModule[task.ModulePath] = task
		}
	}
	var discovered []domain.Task
	for _, sample := range byModule {
		listing := sample
		listing.Args = []string{"run", "--json"}
		if listing.Command == "pnpm" || listing.Command == "yarn" || listing.Command == "bun" {
			listing.Args = []string{"run", "--json"}
		}
		resolution, err := runtimes.ResolveTask(root, listing)
		if err != nil {
			continue
		}
		output, err := runTaskReport(root, resolution.Task, resolution.Environment)
		if err != nil {
			continue
		}
		for _, name := range parseNodeScripts(output) {
			args := []string{"run", name}
			if sample.Command == "pnpm" || sample.Command == "yarn" || sample.Command == "bun" {
				args = []string{name}
			}
			discovered = append(discovered, domain.Task{ID: "node." + sample.Module + "." + name, Name: name, Group: "Package scripts", Adapter: "node", Module: sample.Module, ModulePath: sample.ModulePath, WorkingDir: sample.WorkingDir, Command: sample.Command, Args: args})
		}
	}
	return discovered, nil
}

func parseNodeScripts(output []byte) []string {
	var payload struct {
		Scripts map[string]string `json:"scripts"`
	}
	if json.Unmarshal(output, &payload) != nil || len(payload.Scripts) == 0 {
		return nil
	}
	names := make([]string, 0, len(payload.Scripts))
	for name := range payload.Scripts {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

// discoverMavenTaskReport evaluates each Maven module's effective POM, which
// includes inherited and profile-activated plugin executions. Maven does not
// expose a Gradle-style universal task listing, so configured plugin goals are
// the meaningful dynamically discovered commands.
func discoverMavenTaskReport(root string, _ []domain.Task) ([]domain.Task, error) {
	var pomPaths []string
	err := filepath.WalkDir(root, func(path string, entry os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.IsDir() {
			if ignoredDirectory(entry.Name()) {
				return filepath.SkipDir
			}
			return nil
		}
		if entry.Name() == "pom.xml" {
			pomPaths = append(pomPaths, path)
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	command := "./mvnw"
	if !exists(filepath.Join(root, "mvnw")) && exists(filepath.Join(root, "mvnw.cmd")) {
		command = "mvnw.cmd"
	} else if !exists(filepath.Join(root, "mvnw")) {
		command = "mvn"
	}
	var tasks []domain.Task
	for _, pomPath := range pomPaths {
		relative, err := filepath.Rel(root, filepath.Dir(pomPath))
		if err != nil {
			continue
		}
		relative = filepath.ToSlash(relative)
		args := []string{"-f", filepath.ToSlash(pomPath), "help:effective-pom", "-Dstyle.color=never"}
		resolution, err := runtimes.ResolveTask(root, domain.Task{Adapter: "maven", Command: command, WorkingDir: ".", Args: args})
		if err != nil {
			continue
		}
		output, err := runTaskReport(root, resolution.Task, resolution.Environment)
		if err != nil {
			continue
		}
		moduleTasks, err := parseMavenEffectivePOM(output, root, relative, command)
		if err == nil {
			tasks = append(tasks, moduleTasks...)
		}
	}
	return tasks, nil
}

type effectivePOM struct {
	ArtifactID string `xml:"artifactId"`
	Build      struct {
		Plugins []effectivePlugin `xml:"plugins>plugin"`
	} `xml:"build"`
}

type effectivePlugin struct {
	GroupID    string `xml:"groupId"`
	ArtifactID string `xml:"artifactId"`
	Executions []struct {
		Goals []string `xml:"goals>goal"`
	} `xml:"executions>execution"`
}

func parseMavenEffectivePOM(output []byte, root, relative, command string) ([]domain.Task, error) {
	start := strings.Index(string(output), "<?xml")
	if start < 0 {
		start = strings.Index(string(output), "<project")
	}
	if start < 0 {
		return nil, fmt.Errorf("effective POM was not present in Maven output")
	}
	endOffset := strings.LastIndex(string(output[start:]), "</project>")
	if endOffset < 0 {
		return nil, fmt.Errorf("effective POM did not terminate")
	}
	var pom effectivePOM
	if err := xml.Unmarshal(output[start:start+endOffset+len("</project>")], &pom); err != nil {
		return nil, err
	}
	module, modulePath, err := moduleName(root, filepath.Join(root, relative))
	if err != nil {
		return nil, err
	}
	var tasks []domain.Task
	for _, plugin := range pom.Build.Plugins {
		group := plugin.GroupID
		if group == "" {
			group = "org.apache.maven.plugins"
		}
		for _, execution := range plugin.Executions {
			for _, goal := range execution.Goals {
				invocation := group + ":" + plugin.ArtifactID + ":" + goal
				args := []string{invocation}
				if modulePath != "." {
					args = []string{"-pl", modulePath, invocation}
				}
				prefix := strings.TrimSuffix(plugin.ArtifactID, "-plugin")
				prefix = strings.TrimPrefix(prefix, "maven-")
				prefix = strings.TrimSuffix(prefix, "-maven")
				name := prefix + ":" + goal
				id := "maven." + module + "." + strings.NewReplacer(":", ".", "-", "_").Replace(name)
				tasks = append(tasks, domain.Task{ID: id, Name: name, Group: "Plugin tasks", Adapter: "maven", Module: module, ModulePath: modulePath, WorkingDir: ".", Command: command, Args: args})
			}
		}
	}
	if hasSpringBootPlugin(pom.Build.Plugins) {
		tasks = append(tasks, springBootTasks(module, modulePath, command)...)
	}
	return tasks, nil
}

func hasSpringBootPlugin(plugins []effectivePlugin) bool {
	for _, plugin := range plugins {
		if plugin.ArtifactID == "spring-boot-maven-plugin" && (plugin.GroupID == "" || plugin.GroupID == "org.springframework.boot") {
			return true
		}
	}
	return false
}

func springBootTasks(module, modulePath, command string) []domain.Task {
	baseArgs := func(extra ...string) []string {
		args := append([]string{}, extra...)
		if modulePath != "." {
			args = append([]string{"-pl", modulePath}, args...)
		}
		return args
	}
	const invocation = "org.springframework.boot:spring-boot-maven-plugin:run"
	return []domain.Task{
		{ID: "maven." + module + ".spring-boot.run", Name: "spring-boot:run", Group: "Spring Boot", Favorite: true, Adapter: "maven", Module: module, ModulePath: modulePath, WorkingDir: ".", Command: command, Args: baseArgs(invocation)},
		{ID: "maven." + module + ".spring-boot.run.local", Name: "spring-boot:run (local)", Group: "Spring Boot", Favorite: true, Adapter: "maven", Module: module, ModulePath: modulePath, WorkingDir: ".", Command: command, Args: baseArgs("-Dspring-boot.run.profiles=local", invocation)},
	}
}

func runTaskReport(root string, task domain.Task, environment map[string]string) ([]byte, error) {
	ctx, cancel := context.WithTimeout(context.Background(), dynamicTaskTimeout)
	defer cancel()
	process := exec.CommandContext(ctx, task.Command, task.Args...)
	process.Dir = root
	process.Env = applyEnvironment(os.Environ(), environment)
	output, err := process.Output()
	if err != nil {
		if ctx.Err() != nil {
			return nil, fmt.Errorf("task report timed out after %s", dynamicTaskTimeout)
		}
		return nil, err
	}
	return output, nil
}
