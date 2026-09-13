// Package execution runs normalized tasks without constructing shell commands.
package execution

import (
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/swsw1005/local-dev-launcher/internal/domain"
	"github.com/swsw1005/local-dev-launcher/internal/runtimes"
)

// Run streams the task process directly to the caller's output streams.
func Run(ctx context.Context, root string, task domain.Task, stdout, stderr io.Writer) error {
	return RunWithOptions(ctx, root, task, stdout, stderr, Options{})
}

type Options struct {
	Env         map[string]string
	PrependArgs []string
	AppendArgs  []string
}

// RunWithOptions applies user profile overrides before directly running a task.
func RunWithOptions(ctx context.Context, root string, task domain.Task, stdout, stderr io.Writer, options Options) error {
	command, err := NewCommand(ctx, root, task, options)
	if err != nil {
		return fmt.Errorf("resolve %s: %w", task.ID, err)
	}
	command.Stdout = stdout
	command.Stderr = stderr
	command.Stdin = os.Stdin
	if err := command.Run(); err != nil {
		return fmt.Errorf("run %s: %w", task.ID, err)
	}
	return nil
}

// NewCommand creates a fully configured direct subprocess for foreground or
// background execution. Callers own its streams and lifecycle.
func NewCommand(ctx context.Context, root string, task domain.Task, options Options) (*exec.Cmd, error) {
	resolved, err := runtimes.ResolveTask(root, task)
	if err != nil {
		return nil, err
	}
	task = resolved.Task
	workingDirectory := root
	if task.WorkingDir != "" && task.WorkingDir != "." {
		workingDirectory = filepath.Join(root, filepath.FromSlash(task.WorkingDir))
	}
	args := append([]string{}, options.PrependArgs...)
	args = append(args, task.Args...)
	args = append(args, options.AppendArgs...)
	command := exec.CommandContext(ctx, task.Command, args...)
	command.Dir = workingDirectory
	command.Env = profileEnvironment(runtimeEnvironment(resolved.Environment), options.Env)
	return command, nil
}

func profileEnvironment(env []string, overrides map[string]string) []string {
	for key, value := range overrides {
		env = setEnv(env, key, os.ExpandEnv(value))
	}
	return env
}

func runtimeEnvironment(overrides map[string]string) []string {
	env := os.Environ()
	for key, value := range overrides {
		env = setEnv(env, key, value)
	}
	return env
}

func setEnv(env []string, key, value string) []string {
	prefix := key + "="
	for index, item := range env {
		if strings.HasPrefix(item, prefix) {
			env[index] = prefix + value
			return env
		}
	}
	return append(env, prefix+value)
}
