// Package gitignore checks whether LDR's local state is excluded by Git.
package gitignore

import (
	"context"
	"errors"
	"os/exec"
	"path/filepath"
)

// Checker isolates Git calls so behavior can be tested without a repository.
type Checker interface {
	InsideWorkTree(ctx context.Context, directory string) (bool, error)
	IsIgnored(ctx context.Context, directory, path string) (bool, error)
}

type CommandChecker struct{}

func (CommandChecker) InsideWorkTree(ctx context.Context, directory string) (bool, error) {
	command := exec.CommandContext(ctx, "git", "rev-parse", "--is-inside-work-tree")
	command.Dir = directory
	output, err := command.Output()
	if err != nil {
		var exitError *exec.ExitError
		if errors.As(err, &exitError) {
			return false, nil
		}
		return false, err
	}
	return string(output) == "true\n", nil
}

func (CommandChecker) IsIgnored(ctx context.Context, directory, path string) (bool, error) {
	command := exec.CommandContext(ctx, "git", "check-ignore", "--quiet", "--", path)
	command.Dir = directory
	err := command.Run()
	if err == nil {
		return true, nil
	}
	var exitError *exec.ExitError
	if errors.As(err, &exitError) && exitError.ExitCode() == 1 {
		return false, nil
	}
	return false, err
}

// Status represents Git safety without treating non-Git directories as errors.
type Status struct {
	InRepository bool
	Ignored      bool
}

func Check(ctx context.Context, checker Checker, root string) (Status, error) {
	inRepository, err := checker.InsideWorkTree(ctx, root)
	if err != nil || !inRepository {
		return Status{InRepository: inRepository}, err
	}
	ignored, err := checker.IsIgnored(ctx, root, filepath.Join(".ldr")+string(filepath.Separator))
	if err != nil {
		return Status{}, err
	}
	return Status{InRepository: true, Ignored: ignored}, nil
}
