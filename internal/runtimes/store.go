// Package runtimes defines the per-user location of shared runtime installs.
package runtimes

import (
	"os"
	"path/filepath"
	"runtime"
)

const (
	appDirectory = "local-dev-runner"
	overrideKey  = "LDR_RUNTIME_HOME"
)

// StoreRoot returns the user-level runtime root. An explicit LDR_RUNTIME_HOME
// is useful for a shared volume and always wins over platform defaults.
func StoreRoot() (string, error) {
	if override := os.Getenv(overrideKey); override != "" {
		return filepath.Clean(override), nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return storeRootFor(home, os.Getenv("XDG_DATA_HOME"), runtime.GOOS), nil
}

func storeRootFor(home, xdgDataHome, goos string) string {
	switch goos {
	case "darwin":
		return filepath.Join(home, "Library", "Application Support", appDirectory, "runtimes")
	case "windows":
		if localAppData := os.Getenv("LOCALAPPDATA"); localAppData != "" {
			return filepath.Join(localAppData, appDirectory, "runtimes")
		}
		return filepath.Join(home, "AppData", "Local", appDirectory, "runtimes")
	default:
		if xdgDataHome != "" {
			return filepath.Join(xdgDataHome, appDirectory, "runtimes")
		}
		return filepath.Join(home, ".local", "share", appDirectory, "runtimes")
	}
}
