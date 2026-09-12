// Package cli exposes the first, non-interactive LDR commands.
package cli

import (
	"context"
	"errors"
	"fmt"
	"io"
	"path/filepath"

	"github.com/swim/local-dev-runner/internal/gitignore"
	"github.com/swim/local-dev-runner/internal/project"
	"github.com/swim/local-dev-runner/internal/state"
)

type App struct {
	out      io.Writer
	errOut   io.Writer
	git      gitignore.Checker
	findRoot func(string) (string, error)
}

func New(out, errOut io.Writer) App {
	return App{out: out, errOut: errOut, git: gitignore.CommandChecker{}, findRoot: project.FindRoot}
}

func (a App) Run(ctx context.Context, args []string, directory string) error {
	if len(args) == 0 {
		fmt.Fprintln(a.out, "Local Dev Runner\n\nUse `ldr init` to initialize project-local LDR state. Task discovery arrives in Milestone 2.")
		return nil
	}
	if len(args) != 1 || args[0] != "init" {
		return errors.New("usage: ldr init")
	}

	root, err := a.findRoot(directory)
	if err != nil {
		return fmt.Errorf("find project root: %w", err)
	}
	layout := state.NewLayout(root)
	if err := layout.Ensure(); err != nil {
		return err
	}
	fmt.Fprintf(a.out, "Initialized %s\n", filepath.Join(root, ".ldr"))

	status, err := gitignore.Check(ctx, a.git, root)
	if err != nil {
		return fmt.Errorf("check Git ignore status: %w", err)
	}
	if status.InRepository && !status.Ignored {
		fmt.Fprintln(a.errOut, "WARNING: .ldr/ is not ignored by Git.\n\nLDR stores generated cache, local process state, and user execution profiles inside .ldr/. Committing this directory is not recommended.\n\nAdd the following entry to .gitignore:\n\n    .ldr/")
	}
	return nil
}
