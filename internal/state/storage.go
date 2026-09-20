package state

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
)

// WriteJSON persists generated state without exposing a partially written file
// to another LDR process. The temporary file lives beside the target so the
// final rename remains atomic on the same filesystem.
func WriteJSON(path string, value any) error {
	contents, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return err
	}
	contents = append(contents, '\n')
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return fmt.Errorf("create state directory: %w", err)
	}
	temporary, err := os.CreateTemp(filepath.Dir(path), ".ldr-*.tmp")
	if err != nil {
		return fmt.Errorf("create temporary state file: %w", err)
	}
	temporaryPath := temporary.Name()
	keepTemporary := true
	defer func() {
		_ = temporary.Close()
		if keepTemporary {
			_ = os.Remove(temporaryPath)
		}
	}()
	if err := temporary.Chmod(0o644); err != nil {
		return fmt.Errorf("set state file permissions: %w", err)
	}
	if _, err := temporary.Write(contents); err != nil {
		return fmt.Errorf("write temporary state file: %w", err)
	}
	if err := temporary.Sync(); err != nil {
		return fmt.Errorf("sync temporary state file: %w", err)
	}
	if err := temporary.Close(); err != nil {
		return fmt.Errorf("close temporary state file: %w", err)
	}
	if err := os.Rename(temporaryPath, path); err != nil {
		return fmt.Errorf("replace state file: %w", err)
	}
	keepTemporary = false
	return nil
}

func ReadJSON[T any](path string) (T, error) {
	var value T
	contents, err := os.ReadFile(path)
	if err != nil {
		return value, err
	}
	if err := json.Unmarshal(contents, &value); err != nil {
		return value, err
	}
	return value, nil
}
