package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"strings"

	"github.com/swsw1005/local-dev-launcher/internal/agentguidance"
	"github.com/swsw1005/local-dev-launcher/internal/cli"
)

func main() {
	guidanceDone := make(chan struct{})
	go func() {
		defer close(guidanceDone)
		if len(os.Args) > 1 && os.Args[1] == "doctor" {
			return
		}
		missing := make([]string, 0, 3)
		for _, file := range agentguidance.DetectedAgentFiles() {
			if !file.HasGuidance && file.Err == nil {
				missing = append(missing, file.Name)
			}
		}
		if len(missing) > 0 {
			fmt.Fprintf(os.Stderr, "ldr: LDR runtime setup is incomplete (%s). Run `ldr doctor --fix-agent-guidance` to create the guide and add missing links.\n", strings.Join(missing, ", "))
		}
	}()

	application := cli.New(os.Stdin, os.Stdout, os.Stderr)
	err := application.Run(context.Background(), os.Args[1:], mustGetwd())
	// Complete the background check before exiting so short commands can show
	// the advisory too. The check never edits files during normal invocation.
	<-guidanceDone
	if err != nil {
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
