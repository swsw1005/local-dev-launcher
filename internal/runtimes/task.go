// Package runtimes resolves a discovered task to the SDK installed in LDR's
// user-level runtime store.
package runtimes

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"

	"github.com/swsw1005/local-dev-launcher/internal/domain"
)

const defaultNodeMajor = 24

// TaskResolution replaces commands which otherwise depend on the caller's
// PATH, and supplies SDK-specific environment variables for child processes.
type TaskResolution struct {
	Task        domain.Task
	Environment map[string]string
}

// ResolveTask selects the matching installed runtime for a task. Runtime
// selection is performed just before execution, so changing a project's
// runtime declaration never requires rediscovering its task cache.
func ResolveTask(root string, task domain.Task) (TaskResolution, error) {
	store, err := StoreRoot()
	if err != nil {
		return TaskResolution{}, fmt.Errorf("locate LDR runtime store: %w", err)
	}
	resolution := TaskResolution{Task: task, Environment: map[string]string{}}
	switch task.Adapter {
	case "node":
		return resolveNodeTask(root, store, resolution)
	case "go":
		return resolveGoTask(root, store, resolution)
	case "gradle", "maven":
		return resolveJavaTask(root, store, resolution)
	default:
		return resolution, nil
	}
}

type nodePackage struct {
	Engines map[string]string `json:"engines"`
}

func resolveNodeTask(root, store string, resolution TaskResolution) (TaskResolution, error) {
	workingDirectory := taskDirectory(root, resolution.Task)
	contents, err := os.ReadFile(filepath.Join(workingDirectory, "package.json"))
	if err != nil {
		return TaskResolution{}, fmt.Errorf("read Node project package.json: %w", err)
	}
	var pkg nodePackage
	if err := json.Unmarshal(contents, &pkg); err != nil {
		return TaskResolution{}, fmt.Errorf("parse Node project package.json: %w", err)
	}
	major, err := resolveNodeMajor(filepath.Join(store, "node"), pkg.Engines["node"])
	if err != nil {
		return TaskResolution{}, err
	}
	nodeBin := filepath.Join(store, "node", strconv.Itoa(major), "bin")
	resolution.Environment["PATH"] = nodeBin + string(os.PathListSeparator) + os.Getenv("PATH")
	commandName := resolution.Task.Command
	switch commandName {
	case "npm", "npx", "node", "corepack":
		resolution.Task.Command = filepath.Join(nodeBin, resolution.Task.Command)
	case "pnpm", "yarn":
		// Corepack is shipped with Node and is a stable way to activate the
		// package-manager version declared by package.json.
		resolution.Task.Command = filepath.Join(nodeBin, "corepack")
		resolution.Task.Args = append([]string{commandName}, resolution.Task.Args...)
	case "bun":
		return TaskResolution{}, errors.New("Bun tasks require a Bun runtime; LDR's Node runtime store cannot run bun")
	}
	if _, err := os.Stat(resolution.Task.Command); err != nil {
		return TaskResolution{}, fmt.Errorf("Node %d is selected but %s is unavailable: %w", major, resolution.Task.Command, err)
	}
	return resolution, nil
}

func resolveNodeMajor(nodeRoot, requirement string) (int, error) {
	available, err := installedNodeMajors(nodeRoot)
	if err != nil {
		return 0, err
	}
	if len(available) == 0 {
		return 0, fmt.Errorf("no Node runtime is installed under %s", nodeRoot)
	}
	if strings.TrimSpace(requirement) == "" {
		if containsMajor(available, defaultNodeMajor) {
			return defaultNodeMajor, nil
		}
		return available[len(available)-1], nil
	}
	for _, major := range available {
		if nodeRequirementMatches(requirement, major) {
			return major, nil
		}
	}
	return 0, fmt.Errorf("no installed Node runtime satisfies engines.node %q (available: %s)", requirement, joinMajors(available))
}

func installedNodeMajors(nodeRoot string) ([]int, error) {
	entries, err := os.ReadDir(nodeRoot)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, nil
		}
		return nil, err
	}
	majors := make([]int, 0, len(entries))
	for _, entry := range entries {
		major, err := strconv.Atoi(entry.Name())
		if err != nil || major < 1 {
			continue
		}
		if info, err := os.Stat(filepath.Join(nodeRoot, entry.Name(), "bin", "node")); err == nil && !info.IsDir() {
			majors = append(majors, major)
		}
	}
	sort.Ints(majors)
	return majors, nil
}

func containsMajor(majors []int, want int) bool {
	for _, major := range majors {
		if major == want {
			return true
		}
	}
	return false
}

func joinMajors(majors []int) string {
	values := make([]string, len(majors))
	for index, major := range majors {
		values[index] = strconv.Itoa(major)
	}
	return strings.Join(values, ", ")
}

var nodeComparator = regexp.MustCompile(`(?i)(\^|~|>=|<=|>|<|=)?\s*v?(\d+)(?:\.\d+|\.x|\.\*)*`)

// nodeRequirementMatches deliberately selects at major-version granularity:
// LDR's Node store exposes major-version paths. It handles conventional npm
// engine ranges while avoiding a misleading patch-level promise.
func nodeRequirementMatches(requirement string, major int) bool {
	for _, alternative := range strings.Split(requirement, "||") {
		comparators := nodeComparator.FindAllStringSubmatch(alternative, -1)
		if len(comparators) == 0 {
			continue
		}
		matched := true
		for _, comparator := range comparators {
			value, _ := strconv.Atoi(comparator[2])
			switch comparator[1] {
			case ">=":
				matched = matched && major >= value
			case ">":
				matched = matched && major > value
			case "<=":
				matched = matched && major <= value
			case "<":
				matched = matched && major < value
			default: // bare, =, ^, and ~ all pin a Node major in this store.
				matched = matched && major == value
			}
		}
		if matched {
			return true
		}
	}
	return false
}

func resolveGoTask(root, store string, resolution TaskResolution) (TaskResolution, error) {
	workingDirectory := taskDirectory(root, resolution.Task)
	requirement, err := ReadProjectGoRequirement(workingDirectory)
	if err != nil {
		return TaskResolution{}, err
	}
	goResolution, err := ResolveGo(store, requirement, installedGoVersion)
	if err != nil {
		return TaskResolution{}, err
	}
	if goResolution.Status != "ready" {
		return TaskResolution{}, fmt.Errorf("no installed Go runtime satisfies %s from %s", requirement.Minimum, requirement.Source)
	}
	resolution.Task.Command = goResolution.Executable
	resolution.Environment["PATH"] = filepath.Dir(goResolution.Executable) + string(os.PathListSeparator) + os.Getenv("PATH")
	return resolution, nil
}

func installedGoVersion(executable string) (GoVersion, error) {
	output, err := exec.Command(executable, "version").Output()
	if err != nil {
		return GoVersion{}, err
	}
	fields := strings.Fields(string(output))
	if len(fields) < 3 {
		return GoVersion{}, fmt.Errorf("unexpected go version output %q", strings.TrimSpace(string(output)))
	}
	return ParseGoVersion(fields[2])
}

func resolveJavaTask(root, store string, resolution TaskResolution) (TaskResolution, error) {
	major := requiredJavaMajor(root, resolution.Task)
	javaHome := filepath.Join(store, "java", strconv.Itoa(major))
	if info, err := os.Stat(filepath.Join(javaHome, "bin", "java")); err != nil || info.IsDir() {
		if err == nil {
			err = errors.New("not an executable")
		}
		return TaskResolution{}, fmt.Errorf("Java %d is selected but unavailable at %s: %w", major, javaHome, err)
	}
	resolution.Environment["JAVA_HOME"] = javaHome
	resolution.Environment["PATH"] = filepath.Join(javaHome, "bin") + string(os.PathListSeparator) + os.Getenv("PATH")
	return resolution, nil
}

var gradleJavaMajorPattern = regexp.MustCompile(`(?m)(?:JavaLanguageVersion\.of\(|VERSION_)(\d{1,2})`)
var mavenJavaMajorPattern = regexp.MustCompile(`(?m)<maven\.compiler\.(?:release|source|target)>\s*(\d{1,2})`)

func requiredJavaMajor(root string, task domain.Task) int {
	paths := []string{
		filepath.Join(root, "gradle.properties"),
		filepath.Join(root, "build.gradle"), filepath.Join(root, "build.gradle.kts"),
		filepath.Join(taskDirectory(root, task), "build.gradle"), filepath.Join(taskDirectory(root, task), "build.gradle.kts"),
		filepath.Join(taskDirectory(root, task), "pom.xml"),
	}
	for _, path := range paths {
		contents, err := os.ReadFile(path)
		if err != nil {
			continue
		}
		matches := gradleJavaMajorPattern.FindStringSubmatch(string(contents))
		if len(matches) != 2 {
			matches = mavenJavaMajorPattern.FindStringSubmatch(string(contents))
		}
		if len(matches) == 2 {
			if major, err := strconv.Atoi(matches[1]); err == nil {
				return major
			}
		}
	}
	return 21
}

func taskDirectory(root string, task domain.Task) string {
	if task.WorkingDir == "" || task.WorkingDir == "." {
		return root
	}
	return filepath.Join(root, filepath.FromSlash(task.WorkingDir))
}
