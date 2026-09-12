package runtimes

import (
	"path/filepath"
	"testing"
)

func TestStoreRootForMacOS(t *testing.T) {
	got := storeRootFor("/Users/example", "", "darwin")
	want := "/Users/example/Library/Application Support/local-dev-runner/runtimes"
	if got != want {
		t.Fatalf("store root = %q, want %q", got, want)
	}
}

func TestStoreRootForLinuxUsesXDGDataHome(t *testing.T) {
	got := storeRootFor("/home/example", "/data", "linux")
	want := filepath.Join("/data", "local-dev-runner", "runtimes")
	if got != want {
		t.Fatalf("store root = %q, want %q", got, want)
	}
}

func TestStoreRootForLinuxFallsBackToLocalShare(t *testing.T) {
	got := storeRootFor("/home/example", "", "linux")
	want := filepath.Join("/home/example", ".local", "share", "local-dev-runner", "runtimes")
	if got != want {
		t.Fatalf("store root = %q, want %q", got, want)
	}
}
