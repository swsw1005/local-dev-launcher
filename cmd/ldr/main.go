package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"

	"github.com/swsw1005/local-dev-launcher/internal/cli"
)

func main() {
	application := cli.New(os.Stdin, os.Stdout, os.Stderr)
	if err := application.Run(context.Background(), os.Args[1:], mustGetwd()); err != nil {
		fmt.Fprintln(os.Stderr, "ldr:", err)
		var exitError *exec.ExitError
		if errors.As(err, &exitError) {
			os.Exit(exitError.ExitCode())
		}
		os.Exit(1)
	}
}

func mustGetwd() string {
	wd, err := os.Getwd()
	if err != nil {
		fmt.Fprintln(os.Stderr, "ldr: determine current directory:", err)
		os.Exit(1)
	}
	return wd
}
