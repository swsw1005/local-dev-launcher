package process

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/swsw1005/local-dev-launcher/internal/domain"
	"github.com/swsw1005/local-dev-launcher/internal/execution"
	"github.com/swsw1005/local-dev-launcher/internal/state"
)

func TestStartPersistsRecordAndLog(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("shell fixture is POSIX only")
	}
	root := t.TempDir()
	script := filepath.Join(root, "server.sh")
	if err := os.WriteFile(script, []byte("#!/bin/sh\nprintf 'started\\n'\nsleep 10\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	layout := state.NewLayout(root)
	if err := layout.Ensure(); err != nil {
		t.Fatal(err)
	}
	manager := New(layout)
	record, err := manager.Start(context.Background(), domain.Task{ID: "test.server", Command: script}, execution.Options{})
	if err != nil {
		t.Fatal(err)
	}
	defer manager.Stop(record.ID)
	if len(manager.List()) != 1 || manager.List()[0].TaskID != "test.server" {
		t.Fatalf("records = %#v", manager.List())
	}
	if _, err := os.Stat(record.LogPath); err != nil {
		t.Fatal(err)
	}
}
