package state

import (
	"os"
	"testing"
)

func TestEnsureCreatesOnlyLDRDirectories(t *testing.T) {
	layout := NewLayout(t.TempDir())
	if err := layout.Ensure(); err != nil {
		t.Fatal(err)
	}
	for _, path := range []string{layout.Base, layout.Cache, layout.Profiles, layout.State} {
		info, err := os.Stat(path)
		if err != nil {
			t.Fatalf("stat %s: %v", path, err)
		}
		if !info.IsDir() {
			t.Fatalf("%s is not a directory", path)
		}
	}
}
