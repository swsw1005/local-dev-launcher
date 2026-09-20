package process

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"testing"
	"time"

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

func TestStopUsesGracefulShutdownAndProcessGroup(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("process groups are POSIX-only")
	}
	root := t.TempDir()
	script := filepath.Join(root, "graceful.sh")
	if err := os.WriteFile(script, []byte("#!/bin/sh\ntrap 'exit 0' TERM\nsleep 30\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	layout := state.NewLayout(root)
	if err := layout.Ensure(); err != nil {
		t.Fatal(err)
	}
	manager := New(layout)
	record, err := manager.Start(context.Background(), domain.Task{ID: "test.graceful", Command: script}, execution.Options{})
	if err != nil {
		t.Fatal(err)
	}
	stopped, err := manager.Stop(record.ID)
	if err != nil {
		t.Fatal(err)
	}
	if stopped.Status != "STOPPED" {
		t.Fatalf("status = %q, want STOPPED", stopped.Status)
	}
	deadline := time.Now().Add(time.Second)
	for time.Now().Before(deadline) && processAlive(record.PID, record.PGID) {
		time.Sleep(10 * time.Millisecond)
	}
	if processAlive(record.PID, record.PGID) {
		t.Fatalf("process group %d is still alive", record.PGID)
	}
}

func TestRestartCreatesNewManagedProcess(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("process groups are POSIX-only")
	}
	root := t.TempDir()
	script := filepath.Join(root, "restart.sh")
	if err := os.WriteFile(script, []byte("#!/bin/sh\nsleep 30\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	layout := state.NewLayout(root)
	if err := layout.Ensure(); err != nil {
		t.Fatal(err)
	}
	manager := New(layout)
	first, err := manager.Start(context.Background(), domain.Task{ID: "test.restart", Command: script}, execution.Options{})
	if err != nil {
		t.Fatal(err)
	}
	second, err := manager.Restart(context.Background(), first.ID, domain.Task{ID: "test.restart", Command: script}, execution.Options{})
	if err != nil {
		t.Fatal(err)
	}
	defer manager.Stop(second.ID)
	if second.ID == first.ID || second.PID == first.PID {
		t.Fatalf("restart did not create a new process: first=%#v second=%#v", first, second)
	}
}

func TestReconcileMarksMissingProcessAsOrphaned(t *testing.T) {
	root := t.TempDir()
	layout := state.NewLayout(root)
	if err := layout.Ensure(); err != nil {
		t.Fatal(err)
	}
	manager := New(layout)
	record := Record{ID: "stale", TaskID: "test.stale", PID: 999999, StartedAt: time.Now().Add(-time.Minute), LogPath: filepath.Join(root, "stale.log"), Status: "RUNNING"}
	if err := state.WriteJSON(filepath.Join(layout.State, "processes.json"), []Record{record}); err != nil {
		t.Fatal(err)
	}
	reconciled := manager.Reconcile()
	if len(reconciled) != 1 || reconciled[0].Status != "ORPHANED" || reconciled[0].FinishedAt == nil {
		t.Fatalf("reconciled = %#v", reconciled)
	}
	if got := manager.List()[0].Status; got != "ORPHANED" {
		t.Fatalf("persisted status = %q", got)
	}
	stopped, err := manager.Stop("stale")
	if err != nil {
		t.Fatal(err)
	}
	if stopped.Status != "STOPPED" {
		t.Fatalf("stopped status = %q", stopped.Status)
	}
}
