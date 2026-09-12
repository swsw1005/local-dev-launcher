package runtimes

import (
	"os"
	"path/filepath"
	"testing"
)

func TestReadGoRequirementReadsMinimumAndPreferredToolchain(t *testing.T) {
	path := filepath.Join(t.TempDir(), "go.mod")
	contents := "module example.com/sample\n\ngo 1.26.4\ntoolchain go1.26.8\n"
	if err := os.WriteFile(path, []byte(contents), 0o644); err != nil {
		t.Fatal(err)
	}

	requirement, err := ReadGoRequirement(path)
	if err != nil {
		t.Fatal(err)
	}
	if got, want := requirement.Minimum.String(), "1.26.4"; got != want {
		t.Fatalf("minimum = %q, want %q", got, want)
	}
	if requirement.Preferred == nil || requirement.Preferred.String() != "1.26.8" {
		t.Fatalf("preferred = %#v, want 1.26.8", requirement.Preferred)
	}
}

func TestReadProjectGoRequirementPrefersWorkspace(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "go.mod"), []byte("module example.com/sample\n\ngo 1.26\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "go.work"), []byte("go 1.27.1\ntoolchain go1.27.1\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	requirement, err := ReadProjectGoRequirement(root)
	if err != nil {
		t.Fatal(err)
	}
	if got, want := requirement.Minimum.String(), "1.27.1"; got != want {
		t.Fatalf("minimum = %q, want %q", got, want)
	}
}

func TestResolveGoAcceptsNewerPatchAndUsesPreferredFamily(t *testing.T) {
	store := t.TempDir()
	preferredExecutable := filepath.Join(store, "go", "1.27", "bin", "go")
	if err := os.MkdirAll(filepath.Dir(preferredExecutable), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(preferredExecutable, nil, 0o755); err != nil {
		t.Fatal(err)
	}
	minimum, err := ParseGoVersion("1.26.4")
	if err != nil {
		t.Fatal(err)
	}
	preferred, err := ParseGoVersion("go1.27.1")
	if err != nil {
		t.Fatal(err)
	}

	resolution, err := ResolveGo(store, GoRequirement{Minimum: minimum, Preferred: &preferred}, func(string) (GoVersion, error) {
		return ParseGoVersion("1.27.1")
	})
	if err != nil {
		t.Fatal(err)
	}
	if resolution.Status != "ready" || !resolution.PreferredAvailable {
		t.Fatalf("resolution = %#v", resolution)
	}
	if got, want := resolution.Resolved.String(), "1.27.1"; got != want {
		t.Fatalf("resolved = %q, want %q", got, want)
	}
}

func TestResolveGoIsUnresolvedWhenInstalledVersionMissesPatchMinimum(t *testing.T) {
	store := t.TempDir()
	executable := filepath.Join(store, "go", "1.26", "bin", "go")
	if err := os.MkdirAll(filepath.Dir(executable), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(executable, nil, 0o755); err != nil {
		t.Fatal(err)
	}
	minimum, err := ParseGoVersion("1.26.8")
	if err != nil {
		t.Fatal(err)
	}

	resolution, err := ResolveGo(store, GoRequirement{Minimum: minimum}, func(string) (GoVersion, error) {
		return ParseGoVersion("1.26.7")
	})
	if err != nil {
		t.Fatal(err)
	}
	if resolution.Status != "unresolved" {
		t.Fatalf("status = %q, want unresolved", resolution.Status)
	}
}
