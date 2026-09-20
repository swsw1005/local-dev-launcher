package discovery

import (
	"bufio"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"github.com/swsw1005/local-dev-launcher/internal/domain"
)

func discoverCargoModule(root, directory string) []domain.Task {
	module, relative, err := moduleName(root, directory)
	if err != nil {
		return nil
	}
	working := relative
	if working == "." {
		working = "."
	}
	return []domain.Task{
		{ID: "cargo." + module + ".build", Name: "build", Favorite: true, Adapter: "cargo", Module: module, ModulePath: relative, WorkingDir: working, Command: "cargo", Args: []string{"build"}},
		{ID: "cargo." + module + ".test", Name: "test", Adapter: "cargo", Module: module, ModulePath: relative, WorkingDir: working, Command: "cargo", Args: []string{"test"}},
		{ID: "cargo." + module + ".run", Name: "run", Adapter: "cargo", Module: module, ModulePath: relative, WorkingDir: working, Command: "cargo", Args: []string{"run"}},
	}
}

var makeTarget = regexp.MustCompile(`^([A-Za-z0-9_.-]+)\s*:`)

func discoverMakeTargets(root, path string) ([]domain.Task, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer file.Close()
	relative, err := filepath.Rel(root, filepath.Dir(path))
	if err != nil {
		return nil, err
	}
	module, _, err := moduleName(root, filepath.Dir(path))
	if err != nil {
		return nil, err
	}
	seen := map[string]bool{}
	var targets []string
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		match := makeTarget.FindStringSubmatch(strings.TrimSpace(scanner.Text()))
		if len(match) != 2 || match[1] == ".PHONY" || strings.Contains(match[1], "%") || seen[match[1]] {
			continue
		}
		seen[match[1]] = true
		targets = append(targets, match[1])
	}
	if err := scanner.Err(); err != nil {
		return nil, err
	}
	sort.Strings(targets)
	tasks := make([]domain.Task, 0, len(targets))
	for _, target := range targets {
		tasks = append(tasks, domain.Task{ID: "make." + module + "." + target, Name: target, Adapter: "make", Module: module, ModulePath: relative, WorkingDir: relative, Command: "make", Args: []string{target}})
	}
	return tasks, nil
}

func discoverComposeTasks(root, path string) []domain.Task {
	directory := filepath.Dir(path)
	module, relative, err := moduleName(root, directory)
	if err != nil {
		return nil
	}
	file := filepath.Base(path)
	return []domain.Task{
		{ID: "compose." + module + ".up", Name: "up", Favorite: true, Adapter: "compose", Module: module, ModulePath: relative, WorkingDir: relative, Command: "docker", Args: []string{"compose", "-f", file, "up", "-d"}},
		{ID: "compose." + module + ".down", Name: "down", Adapter: "compose", Module: module, ModulePath: relative, WorkingDir: relative, Command: "docker", Args: []string{"compose", "-f", file, "down"}},
	}
}
