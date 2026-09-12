package main

import (
	"context"
	"fmt"
	"os"

	"github.com/swim/local-dev-runner/internal/cli"
)

func main() {
	application := cli.New(os.Stdout, os.Stderr)
	if err := application.Run(context.Background(), os.Args[1:], mustGetwd()); err != nil {
		fmt.Fprintln(os.Stderr, "ldr:", err)
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
