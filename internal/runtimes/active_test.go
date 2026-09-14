package runtimes

import (
	"os"
	"path/filepath"
	"testing"
)

func TestActivateReplacesShellSymlinksAndListsActiveRuntime(t *testing.T) {
	store := t.TempDir()
	home := t.TempDir()
	t.Setenv("LDR_RUNTIME_HOME", store)
	t.Setenv("HOME", home)
	runtimePath := filepath.Join(store, "java", "21", "bin")
	if err := os.MkdirAll(runtimePath, 0o755); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"java", "javac"} {
		if err := os.WriteFile(filepath.Join(runtimePath, name), []byte("runtime"), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	bin := filepath.Join(home, "bin")
	if err := os.MkdirAll(bin, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink("/old-runtime/java", filepath.Join(bin, "java")); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink("/old-runtime/javac", filepath.Join(bin, "javac")); err != nil {
		t.Fatal(err)
	}

	activation, err := Activate("java", "21")
	if err != nil {
		t.Fatal(err)
	}
	if len(activation.Links) != 2 {
		t.Fatalf("links = %#v", activation.Links)
	}
	target, err := os.Readlink(filepath.Join(bin, "java"))
	if err != nil {
		t.Fatal(err)
	}
	if want := filepath.Join(store, "java", "21", "bin", "java"); target != want {
		t.Fatalf("java link = %q, want %q", target, want)
	}
	installed, err := ListInstalled()
	if err != nil {
		t.Fatal(err)
	}
	if len(installed) != 1 || !installed[0].Active || installed[0].Family != "21" {
		t.Fatalf("installed = %#v", installed)
	}
}

func TestActivateRefusesToReplaceAnOrdinaryFile(t *testing.T) {
	store := t.TempDir()
	home := t.TempDir()
	t.Setenv("LDR_RUNTIME_HOME", store)
	t.Setenv("HOME", home)
	runtimePath := filepath.Join(store, "go", "1.26", "bin")
	if err := os.MkdirAll(runtimePath, 0o755); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"go", "gofmt"} {
		if err := os.WriteFile(filepath.Join(runtimePath, name), []byte("runtime"), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.MkdirAll(filepath.Join(home, "bin"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(home, "bin", "go"), []byte("user file"), 0o755); err != nil {
		t.Fatal(err)
	}
	if _, err := Activate("go", "1.26"); err == nil {
		t.Fatal("Activate succeeded over a user file")
	}
}

func TestFindArchiveHomeFlattensMacOSJavaHome(t *testing.T) {
	root := t.TempDir()
	java := filepath.Join(root, "jdk-21", "Contents", "Home", "bin", "java")
	if err := os.MkdirAll(filepath.Dir(java), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(java, []byte("runtime"), 0o755); err != nil {
		t.Fatal(err)
	}
	home, err := findArchiveHome(root, filepath.Join("Contents", "Home", "bin", "java"))
	if err != nil {
		t.Fatal(err)
	}
	if want := filepath.Join(root, "jdk-21", "Contents", "Home"); home != want {
		t.Fatalf("home = %q, want %q", home, want)
	}
}

func TestRemoveActiveRuntimeRemovesLinksAndSelection(t *testing.T) {
	store := t.TempDir()
	home := t.TempDir()
	t.Setenv("LDR_RUNTIME_HOME", store)
	t.Setenv("HOME", home)
	runtimePath := filepath.Join(store, "java", "21", "bin")
	if err := os.MkdirAll(runtimePath, 0o755); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"java", "javac"} {
		if err := os.WriteFile(filepath.Join(runtimePath, name), []byte("runtime"), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := Activate("java", "21"); err != nil {
		t.Fatal(err)
	}
	if err := Remove("java", "21"); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Lstat(filepath.Join(home, "bin", "java")); !os.IsNotExist(err) {
		t.Fatalf("java shell link still exists: %v", err)
	}
	installed, err := ListInstalled()
	if err != nil {
		t.Fatal(err)
	}
	if len(installed) != 0 {
		t.Fatalf("installed = %#v, want no removed runtime", installed)
	}
}

func TestRemoveRejectsAPathLikeRuntimeFamily(t *testing.T) {
	if err := Remove("java", "../outside"); err == nil {
		t.Fatal("Remove accepted a path-like family")
	}
}
