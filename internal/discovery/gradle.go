package discovery

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"strings"
	"time"

	"github.com/swsw1005/local-dev-launcher/internal/domain"
	"github.com/swsw1005/local-dev-launcher/internal/runtimes"
)

const gradleTaskTimeout = 45 * time.Second

// discoverGradleTasks asks the root wrapper for all configured task paths.
// Gradle itself is the authority for tasks contributed by plugins and custom
// build logic, unlike the static fallback in discovery.go.
func discoverGradleTasks(root string) ([]domain.Task, error) {
	command := "./gradlew"
	if !exists(filepath.Join(root, "gradlew")) {
		if !exists(filepath.Join(root, "gradlew.bat")) {
			return nil, nil
		}
		command = "gradlew.bat"
	}
	resolution, err := runtimes.ResolveTask(root, domain.Task{Adapter: "gradle", Command: command, WorkingDir: "."})
	if err != nil {
		return nil, err
	}
	ctx, cancel := context.WithTimeout(context.Background(), gradleTaskTimeout)
	defer cancel()
	process := exec.CommandContext(ctx, resolution.Task.Command, "tasks", "--all", "--console=plain")
	process.Dir = root
	process.Env = applyEnvironment(os.Environ(), resolution.Environment)
	output, err := process.Output()
	if err != nil {
		if ctx.Err() != nil {
			return nil, fmt.Errorf("Gradle task report timed out after %s", gradleTaskTimeout)
		}
		return nil, fmt.Errorf("run Gradle task report: %w", err)
	}
	return parseGradleTasks(string(output), command), nil
}

func applyEnvironment(environment []string, overrides map[string]string) []string {
	for key, value := range overrides {
		prefix := key + "="
		found := false
		for index, item := range environment {
			if strings.HasPrefix(item, prefix) {
				environment[index] = prefix + value
				found = true
				break
			}
		}
		if !found {
			environment = append(environment, prefix+value)
		}
	}
	return environment
}

// parseGradleTasks accepts Gradle's stable plain-console task report. A task
// path uses colons between its project path and task name, without a leading
// colon in the report.
func parseGradleTasks(output, command string) []domain.Task {
	var tasks []domain.Task
	group := ""
	for _, rawLine := range strings.Split(output, "\n") {
		line := strings.TrimSpace(rawLine)
		if line == "" || strings.HasPrefix(line, ">") || strings.HasPrefix(line, "BUILD ") || strings.HasPrefix(line, "*") {
			continue
		}
		if isGradleGroupHeading(line) {
			group = line
			continue
		}
		if strings.Trim(line, "-") == "" {
			continue
		}
		name, description := splitGradleTaskLine(line)
		if !isGradleTaskName(name) {
			continue
		}
		module, modulePath, invocation := gradleTaskLocation(name)
		tasks = append(tasks, domain.Task{
			ID:          "gradle." + module + "." + path.Base(strings.ReplaceAll(name, ":", "/")),
			Name:        path.Base(strings.ReplaceAll(name, ":", "/")),
			Group:       group,
			Description: description,
			Adapter:     "gradle",
			Module:      module,
			ModulePath:  modulePath,
			WorkingDir:  ".",
			Command:     command,
			Args:        []string{invocation},
		})
	}
	return tasks
}

func isGradleGroupHeading(line string) bool {
	return strings.HasSuffix(line, "tasks") && !strings.Contains(line, ":") && !strings.Contains(line, " - ")
}

func splitGradleTaskLine(line string) (string, string) {
	parts := strings.SplitN(line, " - ", 2)
	if len(parts) == 1 {
		return line, ""
	}
	return parts[0], parts[1]
}

func isGradleTaskName(name string) bool {
	if name == "" || strings.ContainsAny(name, " \t") {
		return false
	}
	for _, character := range name {
		if (character >= 'a' && character <= 'z') || (character >= 'A' && character <= 'Z') ||
			(character >= '0' && character <= '9') || character == ':' || character == '-' || character == '_' {
			continue
		}
		return false
	}
	return true
}

func gradleTaskLocation(name string) (module, modulePath, invocation string) {
	parts := strings.Split(name, ":")
	if len(parts) == 1 {
		return "root", ".", name
	}
	module = strings.Join(parts[:len(parts)-1], ":")
	modulePath = strings.Join(parts[:len(parts)-1], "/")
	return module, modulePath, ":" + name
}
