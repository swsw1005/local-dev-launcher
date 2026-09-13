package execution

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/swsw1005/local-dev-launcher/internal/domain"
)

func TestRunStreamsTaskOutput(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("shell fixture is POSIX only")
	}
	root := t.TempDir()
	script := filepath.Join(root, "task.sh")
	if err := os.WriteFile(script, []byte("#!/bin/sh\nprintf 'task output\\n'\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	var output strings.Builder
	task := domain.Task{ID: "test.output", WorkingDir: ".", Command: script}
	if err := Run(context.Background(), root, task, &output, &output); err != nil {
		t.Fatal(err)
	}
	if got, want := output.String(), "task output\n"; got != want {
		t.Fatalf("output = %q, want %q", got, want)
	}
}

func TestRunPreservesProcessExitError(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("shell fixture is POSIX only")
	}
	root := t.TempDir()
	script := filepath.Join(root, "fails.sh")
	if err := os.WriteFile(script, []byte("#!/bin/sh\nexit 7\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	err := Run(context.Background(), root, domain.Task{ID: "test.failure", WorkingDir: ".", Command: script}, &strings.Builder{}, &strings.Builder{})
	var exitError *exec.ExitError
	if !errors.As(err, &exitError) || exitError.ExitCode() != 7 {
		t.Fatalf("error = %v, want exit code 7", err)
	}
}
