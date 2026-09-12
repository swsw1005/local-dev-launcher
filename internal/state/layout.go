// Package state owns LDR's project-local directory layout.
package state

import (
	"fmt"
	"os"
	"path/filepath"
)

const directoryName = ".ldr"

// Layout keeps generated state separate from user-managed profiles.
type Layout struct {
	Root     string
	Base     string
	Cache    string
	Profiles string
	State    string
}

func NewLayout(root string) Layout {
	base := filepath.Join(root, directoryName)
	return Layout{
		Root:     root,
		Base:     base,
		Cache:    filepath.Join(base, "cache"),
		Profiles: filepath.Join(base, "profiles"),
		State:    filepath.Join(base, "state"),
	}
}

// Ensure creates only directories owned by LDR. It never writes user profiles.
func (l Layout) Ensure() error {
	for _, path := range []string{l.Cache, l.Profiles, l.State} {
		if err := os.MkdirAll(path, 0o755); err != nil {
			return fmt.Errorf("create %s: %w", path, err)
		}
	}
	return nil
}
