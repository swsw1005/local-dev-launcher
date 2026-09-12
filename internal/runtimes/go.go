package runtimes

import (
	"bufio"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

// GoRequirement captures the toolchain constraints in a go.mod or go.work
// file. Minimum is a hard constraint; Preferred is the optional toolchain
// suggestion introduced in Go 1.21.
type GoRequirement struct {
	Minimum   GoVersion
	Preferred *GoVersion
	Source    string
}

// GoVersion is a released Go language or toolchain version. A missing patch
// means the language family (for example, 1.26), whose minimum release is
// treated as 1.26.0 for comparison.
type GoVersion struct {
	Major    int
	Minor    int
	Patch    int
	HasPatch bool
}

func (v GoVersion) String() string {
	if !v.HasPatch {
		return fmt.Sprintf("%d.%d", v.Major, v.Minor)
	}
	return fmt.Sprintf("%d.%d.%d", v.Major, v.Minor, v.Patch)
}

func (v GoVersion) Family() string {
	return fmt.Sprintf("%d.%d", v.Major, v.Minor)
}

// AtLeast reports whether v meets a module's minimum Go requirement.
func (v GoVersion) AtLeast(other GoVersion) bool {
	if v.Major != other.Major {
		return v.Major > other.Major
	}
	if v.Minor != other.Minor {
		return v.Minor > other.Minor
	}
	return v.Patch >= other.Patch
}

// ParseGoVersion accepts module versions (1.26.8) and toolchain names
// (go1.26.8). Release candidates are intentionally reported as unsupported in
// the MVP instead of being incorrectly ordered as stable releases.
func ParseGoVersion(value string) (GoVersion, error) {
	value = strings.TrimPrefix(strings.TrimSpace(value), "go")
	parts := strings.Split(value, ".")
	if len(parts) != 2 && len(parts) != 3 {
		return GoVersion{}, fmt.Errorf("invalid Go version %q", value)
	}
	major, err := strconv.Atoi(parts[0])
	if err != nil || major < 1 {
		return GoVersion{}, fmt.Errorf("invalid Go major version %q", value)
	}
	minor, err := strconv.Atoi(parts[1])
	if err != nil || minor < 0 {
		return GoVersion{}, fmt.Errorf("invalid Go minor version %q", value)
	}
	version := GoVersion{Major: major, Minor: minor}
	if len(parts) == 3 {
		patch, err := strconv.Atoi(parts[2])
		if err != nil || patch < 0 {
			return GoVersion{}, fmt.Errorf("invalid Go patch version %q", value)
		}
		version.Patch = patch
		version.HasPatch = true
	}
	return version, nil
}

// ReadGoRequirement reads top-level go and toolchain directives. A go.work
// requirement is preferred over go.mod because Go itself applies workspace
// constraints first.
func ReadGoRequirement(path string) (GoRequirement, error) {
	file, err := os.Open(path)
	if err != nil {
		return GoRequirement{}, err
	}
	defer file.Close()

	requirement := GoRequirement{Source: path}
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		fields := strings.Fields(strings.TrimSpace(strings.SplitN(scanner.Text(), "//", 2)[0]))
		if len(fields) != 2 {
			continue
		}
		switch fields[0] {
		case "go":
			version, err := ParseGoVersion(fields[1])
			if err != nil {
				return GoRequirement{}, fmt.Errorf("parse %s go directive: %w", path, err)
			}
			requirement.Minimum = version
		case "toolchain":
			if fields[1] == "default" {
				continue
			}
			version, err := ParseGoVersion(fields[1])
			if err != nil {
				return GoRequirement{}, fmt.Errorf("parse %s toolchain directive: %w", path, err)
			}
			requirement.Preferred = &version
		}
	}
	if err := scanner.Err(); err != nil {
		return GoRequirement{}, err
	}
	if requirement.Minimum.Major == 0 {
		return GoRequirement{}, fmt.Errorf("%s does not declare a Go version", path)
	}
	return requirement, nil
}

// ReadProjectGoRequirement applies Go's workspace-first selection order.
func ReadProjectGoRequirement(root string) (GoRequirement, error) {
	for _, name := range []string{"go.work", "go.mod"} {
		path := filepath.Join(root, name)
		requirement, err := ReadGoRequirement(path)
		if err == nil {
			return requirement, nil
		}
		if !errors.Is(err, os.ErrNotExist) {
			return GoRequirement{}, err
		}
	}
	return GoRequirement{}, errors.New("no go.work or go.mod found")
}

// GoVersionReader inspects the exact version of an installed Go executable.
// It keeps subprocess execution outside parsing and selection logic.
type GoVersionReader func(executable string) (GoVersion, error)

// GoResolution is the resolver's deterministic result for an installed Go
// toolchain. PreferredAvailable reports when a toolchain directive could not
// be selected but the hard minimum remains usable.
type GoResolution struct {
	Requirement        GoRequirement
	Resolved           GoVersion
	Executable         string
	Status             string
	PreferredAvailable bool
}

// ResolveGo selects the preferred family when present, otherwise the minimum
// family. A newer installed patch satisfies an older patch-level requirement.
func ResolveGo(storeRoot string, requirement GoRequirement, readVersion GoVersionReader) (GoResolution, error) {
	families := []string{requirement.Minimum.Family()}
	if requirement.Preferred != nil && requirement.Preferred.Family() != families[0] {
		families = append([]string{requirement.Preferred.Family()}, families...)
	}

	for _, family := range families {
		executable := filepath.Join(storeRoot, "go", family, "bin", "go")
		if _, err := os.Stat(executable); err != nil {
			if errors.Is(err, os.ErrNotExist) {
				continue
			}
			return GoResolution{}, err
		}
		resolved, err := readVersion(executable)
		if err != nil {
			return GoResolution{}, err
		}
		if !resolved.AtLeast(requirement.Minimum) {
			continue
		}
		preferredAvailable := requirement.Preferred == nil || resolved.AtLeast(*requirement.Preferred)
		return GoResolution{
			Requirement:        requirement,
			Resolved:           resolved,
			Executable:         executable,
			Status:             "ready",
			PreferredAvailable: preferredAvailable,
		}, nil
	}
	return GoResolution{Requirement: requirement, Status: "unresolved"}, nil
}
