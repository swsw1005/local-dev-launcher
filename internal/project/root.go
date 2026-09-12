// Package project detects the directory that owns LDR state.
package project

import (
	"errors"
	"os"
	"path/filepath"
)

var markers = []string{
	".git",
	"gradlew", "gradlew.bat", "settings.gradle", "settings.gradle.kts", "build.gradle", "build.gradle.kts",
	"mvnw", "mvnw.cmd", "pom.xml",
	"package.json",
}

// FindRoot returns the closest ancestor that looks like a project root. If no
// marker is found, the cleaned starting directory itself is the root.
func FindRoot(start string) (string, error) {
	if start == "" {
		return "", errors.New("project path is empty")
	}
	abs, err := filepath.Abs(start)
	if err != nil {
		return "", err
	}
	abs = filepath.Clean(abs)

	for current := abs; ; current = filepath.Dir(current) {
		for _, marker := range markers {
			if _, err := os.Lstat(filepath.Join(current, marker)); err == nil {
				return current, nil
			}
		}
		parent := filepath.Dir(current)
		if parent == current {
			return abs, nil
		}
	}
}
