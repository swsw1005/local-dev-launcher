package runtimes

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/swsw1005/local-dev-launcher/internal/state"
)

// Installation is one runtime family available to LDR's resolver.
type Installation struct {
	Runtime string
	Family  string
	Path    string
	Active  bool
}

// Activation records the shell links owned by LDR for an active family.
type Activation struct {
	Runtime string
	Family  string
	Links   []string
}

type activeState map[string]activeSelection

type activeSelection struct {
	Family    string    `json:"family"`
	UpdatedAt time.Time `json:"updatedAt"`
}

// ListInstalled reports all ready runtime families and whether each is the
// shell-active selection made through LDR.
func ListInstalled() ([]Installation, error) {
	store, err := StoreRoot()
	if err != nil {
		return nil, err
	}
	active, err := readActiveState(store)
	if err != nil {
		return nil, err
	}
	// Persist the reconciliation done by readActiveState so manually removed
	// runtime directories cannot leave a stale active selection behind.
	if _, err := os.Stat(activeStatePath(store)); err == nil {
		if err := writeActiveState(store, active); err != nil {
			return nil, err
		}
	} else if !errors.Is(err, os.ErrNotExist) {
		return nil, err
	}
	var installations []Installation
	for _, spec := range runtimeSpecs() {
		entries, err := os.ReadDir(filepath.Join(store, spec.runtime))
		if errors.Is(err, os.ErrNotExist) {
			continue
		}
		if err != nil {
			return nil, err
		}
		for _, entry := range entries {
			if !entry.IsDir() {
				continue
			}
			path := filepath.Join(store, spec.runtime, entry.Name())
			if _, err := os.Stat(filepath.Join(path, spec.executables[0])); err != nil {
				continue
			}
			selection := active[spec.runtime]
			installations = append(installations, Installation{Runtime: spec.runtime, Family: entry.Name(), Path: path, Active: selection.Family == entry.Name() && linksMatch(spec, path)})
		}
	}
	sort.Slice(installations, func(i, j int) bool {
		if installations[i].Runtime == installations[j].Runtime {
			return installations[i].Family < installations[j].Family
		}
		return installations[i].Runtime < installations[j].Runtime
	})
	return installations, nil
}

// Activate makes a selected family available from ~/bin. It updates only
// existing symlinks or paths that LDR created; an ordinary user file is never
// overwritten.
func Activate(runtime, family string) (Activation, error) {
	runtime = normalizeRuntime(runtime)
	if err := validateFamily(family); err != nil {
		return Activation{}, err
	}
	spec, ok := runtimeSpecFor(runtime)
	if !ok {
		return Activation{}, fmt.Errorf("unsupported runtime %q", runtime)
	}
	store, err := StoreRoot()
	if err != nil {
		return Activation{}, err
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return Activation{}, err
	}
	runtimePath := filepath.Join(store, runtime, family)
	for _, executable := range spec.executables {
		if info, err := os.Stat(filepath.Join(runtimePath, executable)); err != nil || info.IsDir() {
			if err == nil {
				err = errors.New("not an executable")
			}
			return Activation{}, fmt.Errorf("%s %s is not installed: %w", runtime, family, err)
		}
	}
	bin := filepath.Join(home, "bin")
	if err := os.MkdirAll(bin, 0o755); err != nil {
		return Activation{}, err
	}
	links := make([]string, 0, len(spec.executables))
	for _, executable := range spec.executables {
		link := filepath.Join(bin, filepath.Base(executable))
		if info, err := os.Lstat(link); err == nil {
			if info.Mode()&os.ModeSymlink == 0 {
				return Activation{}, fmt.Errorf("refusing to replace non-symlink %s", link)
			}
			if err := os.Remove(link); err != nil {
				return Activation{}, err
			}
		} else if !errors.Is(err, os.ErrNotExist) {
			return Activation{}, err
		}
		target := filepath.Join(runtimePath, executable)
		if err := os.Symlink(target, link); err != nil {
			return Activation{}, err
		}
		links = append(links, link)
	}
	active, err := readActiveState(store)
	if err != nil {
		return Activation{}, err
	}
	active[runtime] = activeSelection{Family: family, UpdatedAt: time.Now().UTC()}
	if err := writeActiveState(store, active); err != nil {
		return Activation{}, err
	}
	return Activation{Runtime: runtime, Family: family, Links: links}, nil
}

// Remove deletes one installed family. If it is active, its LDR shell links
// and selection record are removed as well. The target is always resolved
// below LDR's runtime store.
func Remove(runtime, family string) error {
	runtime = normalizeRuntime(runtime)
	if err := validateFamily(family); err != nil {
		return err
	}
	spec, ok := runtimeSpecFor(runtime)
	if !ok {
		return fmt.Errorf("unsupported runtime %q", runtime)
	}
	store, err := StoreRoot()
	if err != nil {
		return err
	}
	target := filepath.Join(store, runtime, family)
	if info, err := os.Stat(target); err != nil || !info.IsDir() {
		if errors.Is(err, os.ErrNotExist) {
			return fmt.Errorf("%s %s is not installed", runtime, family)
		}
		return err
	}
	active, err := readActiveState(store)
	if err != nil {
		return err
	}
	if active[runtime].Family == family {
		home, err := os.UserHomeDir()
		if err != nil {
			return err
		}
		for _, executable := range spec.executables {
			link := filepath.Join(home, "bin", filepath.Base(executable))
			targetLink, err := os.Readlink(link)
			if err == nil && filepath.Clean(targetLink) == filepath.Join(target, executable) {
				if err := os.Remove(link); err != nil {
					return err
				}
			}
		}
		delete(active, runtime)
		if err := writeActiveState(store, active); err != nil {
			return err
		}
	}
	if err := os.RemoveAll(target); err != nil {
		return err
	}
	return nil
}

type runtimeSpec struct {
	runtime     string
	executables []string
}

func runtimeSpecs() []runtimeSpec {
	return []runtimeSpec{
		{runtime: "java", executables: []string{"bin/java", "bin/javac"}},
		{runtime: "node", executables: []string{"bin/node", "bin/npm", "bin/npx", "bin/pnpm"}},
		{runtime: "go", executables: []string{"bin/go", "bin/gofmt"}},
	}
}

func runtimeSpecFor(runtime string) (runtimeSpec, bool) {
	for _, spec := range runtimeSpecs() {
		if spec.runtime == runtime {
			return spec, true
		}
	}
	return runtimeSpec{}, false
}

func normalizeRuntime(runtime string) string {
	if runtime == "golang" {
		return "go"
	}
	return runtime
}

func validateFamily(family string) error {
	if family == "" || family == "." || family == ".." || filepath.Base(family) != family || strings.ContainsAny(family, `/\\`) {
		return fmt.Errorf("invalid runtime family %q", family)
	}
	return nil
}

func activeStatePath(store string) string { return filepath.Join(store, "active.json") }

func readActiveState(store string) (activeState, error) {
	contents, err := os.ReadFile(activeStatePath(store))
	if errors.Is(err, os.ErrNotExist) {
		return activeState{}, nil
	}
	if err != nil {
		return nil, err
	}
	var state activeState
	if err := json.Unmarshal(contents, &state); err != nil {
		return nil, err
	}
	for runtime, selection := range state {
		if _, err := os.Stat(filepath.Join(store, runtime, selection.Family)); errors.Is(err, os.ErrNotExist) {
			delete(state, runtime)
		}
	}
	return state, nil
}

func writeActiveState(store string, selection activeState) error {
	return state.WriteJSON(activeStatePath(store), selection)
}

func linksMatch(spec runtimeSpec, runtimePath string) bool {
	home, err := os.UserHomeDir()
	if err != nil {
		return false
	}
	for _, executable := range spec.executables {
		link := filepath.Join(home, "bin", filepath.Base(executable))
		target, err := os.Readlink(link)
		if err != nil || filepath.Clean(target) != filepath.Join(runtimePath, executable) {
			return false
		}
	}
	return true
}
