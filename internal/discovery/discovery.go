// Package discovery finds runnable development tasks and caches the normalized
// registry below the project's .ldr/cache directory.
package discovery

import (
	"crypto/sha256"
	"encoding/json"
	"encoding/xml"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/swsw1005/local-dev-launcher/internal/domain"
	"github.com/swsw1005/local-dev-launcher/internal/project"
	"github.com/swsw1005/local-dev-launcher/internal/state"
)

const cacheVersion = 6
const dynamicTaskTimeout = 45 * time.Second

type Source struct {
	Path     string `json:"path"`
	Adapter  string `json:"adapter"`
	Checksum string `json:"checksum"`
}

type Manifest struct {
	Version int      `json:"version"`
	Sources []Source `json:"sources"`
}

type Registry struct {
	Version int           `json:"version"`
	Tasks   []domain.Task `json:"tasks"`
}

type Result struct {
	Tasks  []domain.Task
	Cached bool
}

// LoadOrDiscover reuses cache only when every discovery source is unchanged.
func LoadOrDiscover(root string, layout state.Layout, force bool) (Result, error) {
	sources, err := Sources(root)
	if err != nil {
		return Result{}, err
	}
	if !force {
		manifest, err := state.ReadJSON[Manifest](filepath.Join(layout.Cache, "manifest.json"))
		if err == nil && manifest.Version == cacheVersion && sameSources(manifest.Sources, sources) {
			registry, err := state.ReadJSON[Registry](filepath.Join(layout.Cache, "tasks.json"))
			if err == nil && registry.Version == cacheVersion {
				return Result{Tasks: registry.Tasks, Cached: true}, nil
			}
		}
	}

	tasks, err := DiscoverWithDynamicTools(root)
	if err != nil {
		return Result{}, err
	}
	if err := state.WriteJSON(filepath.Join(layout.Cache, "manifest.json"), Manifest{Version: cacheVersion, Sources: sources}); err != nil {
		return Result{}, err
	}
	if err := state.WriteJSON(filepath.Join(layout.Cache, "tasks.json"), Registry{Version: cacheVersion, Tasks: tasks}); err != nil {
		return Result{}, err
	}
	return Result{Tasks: tasks}, nil
}

// DiscoverWithDynamicTools enriches static discovery with reports from the
// project's build tools. A failed report (for example, a missing private
// repository credential) does not make the launcher unusable: static tasks
// remain available.
func DiscoverWithDynamicTools(root string) ([]domain.Task, error) {
	tasks, err := Discover(root)
	if err != nil {
		return nil, err
	}
	for _, discover := range []func(string, []domain.Task) ([]domain.Task, error){discoverGradleTaskReport, discoverNodeTaskReport, discoverMavenTaskReport} {
		dynamic, err := discover(root, tasks)
		if err != nil {
			continue
		}
		tasks = mergeTasks(tasks, dynamic)
	}
	return tasks, nil
}

// DiscoverWithGradle is retained for callers compiled against the v0.2 API.
func DiscoverWithGradle(root string) ([]domain.Task, error) { return DiscoverWithDynamicTools(root) }

func mergeTasks(static, dynamic []domain.Task) []domain.Task {
	byID := make(map[string]domain.Task, len(static)+len(dynamic))
	for _, task := range static {
		byID[task.ID] = task
	}
	for _, task := range dynamic {
		if fallback, exists := byID[task.ID]; exists && fallback.Favorite {
			task.Favorite = true
		}
		byID[task.ID] = task
	}
	tasks := make([]domain.Task, 0, len(byID))
	for _, task := range byID {
		tasks = append(tasks, task)
	}
	sort.Slice(tasks, func(i, j int) bool { return tasks[i].ID < tasks[j].ID })
	return tasks
}

// Sources returns checksummed files whose changes can affect discovery.
func Sources(root string) ([]Source, error) {
	var sources []Source
	config, err := project.LoadConfig(root)
	if err != nil {
		return nil, err
	}
	configPath := filepath.Join(root, ".ldr", "project.toml")
	if contents, err := os.ReadFile(configPath); err == nil {
		digest := sha256.Sum256(contents)
		sources = append(sources, Source{Path: ".ldr/project.toml", Adapter: "project", Checksum: fmt.Sprintf("sha256:%x", digest)})
	}
	err = filepath.WalkDir(root, func(path string, entry os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.IsDir() {
			if ignoredDirectory(entry.Name(), config.Ignore...) {
				return filepath.SkipDir
			}
			return nil
		}
		adapter := adapterFor(filepath.Base(path))
		if adapter == "" {
			return nil
		}
		contents, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		relative, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		digest := sha256.Sum256(contents)
		sources = append(sources, Source{Path: filepath.ToSlash(relative), Adapter: adapter, Checksum: fmt.Sprintf("sha256:%x", digest)})
		return nil
	})
	if err != nil {
		return nil, err
	}
	sort.Slice(sources, func(i, j int) bool { return sources[i].Path < sources[j].Path })
	return sources, nil
}

func adapterFor(name string) string {
	switch name {
	case "settings.gradle", "settings.gradle.kts", "build.gradle", "build.gradle.kts", "gradlew", "gradlew.bat":
		return "gradle"
	case "pom.xml", "mvnw", "mvnw.cmd":
		return "maven"
	case "package.json", "package-lock.json", "pnpm-lock.yaml", "yarn.lock", "bun.lock":
		return "node"
	case "go.mod", "go.work":
		return "go"
	case "Cargo.toml":
		return "cargo"
	case "Makefile", "makefile":
		return "make"
	case "docker-compose.yml", "docker-compose.yaml", "compose.yml", "compose.yaml":
		return "compose"
	default:
		return ""
	}
}

func sameSources(left, right []Source) bool {
	if len(left) != len(right) {
		return false
	}
	for index := range left {
		if left[index] != right[index] {
			return false
		}
	}
	return true
}

// Discover normalizes Gradle, Maven, Node, and Go tasks without executing a
// build tool. Tool-specific dynamic task discovery can extend this registry.
func Discover(root string) ([]domain.Task, error) {
	var tasks []domain.Task
	config, err := project.LoadConfig(root)
	if err != nil {
		return nil, err
	}
	err = filepath.WalkDir(root, func(path string, entry os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.IsDir() {
			if ignoredDirectory(entry.Name(), config.Ignore...) {
				return filepath.SkipDir
			}
			return nil
		}
		name := entry.Name()
		switch name {
		case "build.gradle", "build.gradle.kts":
			found, err := discoverGradleModule(root, filepath.Dir(path), path)
			if err != nil {
				return err
			}
			tasks = append(tasks, found...)
		case "package.json":
			found, err := discoverNodeModule(root, filepath.Dir(path), path)
			if err != nil {
				return err
			}
			tasks = append(tasks, found...)
		case "pom.xml":
			found, err := discoverMavenModule(root, filepath.Dir(path), path)
			if err != nil {
				return err
			}
			tasks = append(tasks, found...)
		case "go.mod":
			tasks = append(tasks, discoverGoModule(root, filepath.Dir(path))...)
		case "Cargo.toml":
			tasks = append(tasks, discoverCargoModule(root, filepath.Dir(path))...)
		case "Makefile", "makefile":
			found, err := discoverMakeTargets(root, path)
			if err != nil {
				return err
			}
			tasks = append(tasks, found...)
		case "docker-compose.yml", "docker-compose.yaml", "compose.yml", "compose.yaml":
			tasks = append(tasks, discoverComposeTasks(root, path)...)
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	sort.Slice(tasks, func(i, j int) bool { return tasks[i].ID < tasks[j].ID })
	return tasks, nil
}

func discoverGradleModule(root, directory, buildFile string) ([]domain.Task, error) {
	contents, err := os.ReadFile(buildFile)
	if err != nil {
		return nil, err
	}
	module, relative, err := moduleName(root, directory)
	if err != nil {
		return nil, err
	}
	command := "gradle"
	if exists(filepath.Join(root, "gradlew")) {
		command = "./gradlew"
	} else if exists(filepath.Join(root, "gradlew.bat")) {
		command = "gradlew.bat"
	}
	tasks := []domain.Task{}
	for _, name := range []string{"clean", "build", "test"} {
		tasks = append(tasks, gradleTask(module, relative, command, name))
	}
	if strings.Contains(string(contents), "org.springframework.boot") {
		tasks = append(tasks, gradleTask(module, relative, command, "bootRun"))
	}
	return tasks, nil
}

func gradleTask(module, relative, command, name string) domain.Task {
	path := name
	if module != "root" {
		path = ":" + relative + ":" + name
	}
	modulePath := relative
	return domain.Task{ID: "gradle." + module + "." + name, Name: name, Favorite: true, Adapter: "gradle", Module: module, ModulePath: modulePath, WorkingDir: ".", Command: command, Args: []string{path}}
}

type packageFile struct {
	Name           string            `json:"name"`
	Scripts        map[string]string `json:"scripts"`
	PackageManager string            `json:"packageManager"`
}

func discoverNodeModule(root, directory, packagePath string) ([]domain.Task, error) {
	contents, err := os.ReadFile(packagePath)
	if err != nil {
		return nil, err
	}
	var pkg packageFile
	if err := json.Unmarshal(contents, &pkg); err != nil {
		return nil, fmt.Errorf("parse %s: %w", packagePath, err)
	}
	module, relative, err := moduleName(root, directory)
	if err != nil {
		return nil, err
	}
	command := nodeCommand(root, directory, pkg.PackageManager)
	keys := make([]string, 0, len(pkg.Scripts))
	for name := range pkg.Scripts {
		keys = append(keys, name)
	}
	sort.Strings(keys)
	tasks := make([]domain.Task, 0, len(keys))
	for _, name := range keys {
		args := []string{"run", name}
		if command == "pnpm" || command == "yarn" || command == "bun" {
			args = []string{name}
		}
		tasks = append(tasks, domain.Task{ID: "node." + module + "." + name, Name: name, Adapter: "node", Module: module, ModulePath: relative, WorkingDir: relative, Command: command, Args: args})
	}
	return tasks, nil
}

func nodeCommand(root, directory, packageManager string) string {
	if packageManager != "" {
		return strings.Split(packageManager, "@")[0]
	}
	for _, lock := range []struct{ file, command string }{{"pnpm-lock.yaml", "pnpm"}, {"yarn.lock", "yarn"}, {"bun.lock", "bun"}} {
		if exists(filepath.Join(directory, lock.file)) || (directory != root && exists(filepath.Join(root, lock.file))) {
			return lock.command
		}
	}
	return "npm"
}

type pomFile struct {
	ArtifactID string `xml:"artifactId"`
	Build      struct {
		Plugins []effectivePlugin `xml:"plugins>plugin"`
	} `xml:"build"`
}

func discoverMavenModule(root, directory, pomPath string) ([]domain.Task, error) {
	contents, err := os.ReadFile(pomPath)
	if err != nil {
		return nil, err
	}
	var pom pomFile
	if err := xml.Unmarshal(contents, &pom); err != nil {
		return nil, fmt.Errorf("parse %s: %w", pomPath, err)
	}
	module, relative, err := moduleName(root, directory)
	if err != nil {
		return nil, err
	}
	if pom.ArtifactID != "" && module == "root" {
		module = pom.ArtifactID
	}
	command := "mvn"
	if exists(filepath.Join(root, "mvnw")) {
		command = "./mvnw"
	} else if exists(filepath.Join(root, "mvnw.cmd")) {
		command = "mvnw.cmd"
	}
	var tasks []domain.Task
	for _, name := range []string{"clean", "compile", "test", "package", "verify", "install"} {
		args := []string{name}
		if relative != "." {
			args = []string{"-pl", filepath.ToSlash(relative), name}
		}
		tasks = append(tasks, domain.Task{ID: "maven." + module + "." + name, Name: name, Adapter: "maven", Module: module, ModulePath: relative, WorkingDir: ".", Command: command, Args: args})
	}
	if hasSpringBootPlugin(pom.Build.Plugins) {
		tasks = append(tasks, springBootTasks(module, relative, command)...)
	}
	return tasks, nil
}

func discoverGoModule(root, directory string) []domain.Task {
	module, relative, err := moduleName(root, directory)
	if err != nil {
		return nil
	}
	return []domain.Task{
		{ID: "go." + module + ".build", Name: "build", Adapter: "go", Module: module, ModulePath: relative, WorkingDir: relative, Command: "go", Args: []string{"build", "./..."}},
		{ID: "go." + module + ".test", Name: "test", Adapter: "go", Module: module, ModulePath: relative, WorkingDir: relative, Command: "go", Args: []string{"test", "./..."}},
	}
}

func moduleName(root, directory string) (name, relative string, err error) {
	relative, err = filepath.Rel(root, directory)
	if err != nil {
		return "", "", err
	}
	relative = filepath.ToSlash(relative)
	if relative == "." {
		return "root", relative, nil
	}
	return strings.ReplaceAll(relative, "/", "."), relative, nil
}

func exists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

func ignoredDirectory(name string, custom ...string) bool {
	switch name {
	case ".git", ".gradle", ".ldr", ".next", ".worktrees", "node_modules", "build", "dist", "target":
		return true
	}
	for _, ignored := range custom {
		if name == ignored {
			return true
		}
	}
	return false
}
