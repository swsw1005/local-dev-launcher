// Package process persists minimal metadata for LDR-managed background tasks.
package process

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/swsw1005/local-dev-launcher/internal/domain"
	"github.com/swsw1005/local-dev-launcher/internal/execution"
	"github.com/swsw1005/local-dev-launcher/internal/state"
)

type Record struct {
	ID         string     `json:"id"`
	TaskID     string     `json:"taskId"`
	PID        int        `json:"pid"`
	PGID       int        `json:"pgid,omitempty"`
	StartedAt  time.Time  `json:"startedAt"`
	FinishedAt *time.Time `json:"finishedAt,omitempty"`
	ExitCode   *int       `json:"exitCode,omitempty"`
	Signal     string     `json:"signal,omitempty"`
	LogPath    string     `json:"logPath"`
	Status     string     `json:"status"`
}

type Manager struct{ layout state.Layout }

func New(layout state.Layout) Manager { return Manager{layout: layout} }

func (m Manager) Start(ctx context.Context, task domain.Task, options execution.Options) (Record, error) {
	logs := filepath.Join(m.layout.State, "logs")
	if err := os.MkdirAll(logs, 0o755); err != nil {
		return Record{}, err
	}
	id := fmt.Sprintf("%s-%d", safeID(task.ID), time.Now().UnixNano())
	logPath := filepath.Join(logs, id+".log")
	logFile, err := os.OpenFile(logPath, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		return Record{}, err
	}
	command, err := execution.NewCommand(ctx, m.layout.Root, task, options)
	if err != nil {
		logFile.Close()
		return Record{}, fmt.Errorf("resolve %s: %w", task.ID, err)
	}
	command.Stdout = logFile
	command.Stderr = logFile
	command.Stdin = nil
	configureProcessGroup(command)
	if err := command.Start(); err != nil {
		logFile.Close()
		return Record{}, fmt.Errorf("start %s: %w", task.ID, err)
	}
	if err := logFile.Close(); err != nil {
		_ = terminateProcess(command.Process.Pid, processGroupID(command.Process.Pid), true)
		return Record{}, err
	}
	record := Record{ID: id, TaskID: task.ID, PID: command.Process.Pid, PGID: processGroupID(command.Process.Pid), StartedAt: time.Now().UTC(), LogPath: logPath, Status: "RUNNING"}
	if err := m.save(append(m.List(), record)); err != nil {
		_ = terminateProcess(record.PID, record.PGID, true)
		return Record{}, err
	}
	go m.wait(command, record.ID)
	return record, nil
}

func (m Manager) wait(command *exec.Cmd, id string) {
	err := command.Wait()
	records := m.List()
	for index := range records {
		if records[index].ID != id {
			continue
		}
		finished := time.Now().UTC()
		records[index].FinishedAt = &finished
		if err == nil {
			records[index].Status = "EXITED"
		} else {
			records[index].Status = "FAILED"
			if exitErr, ok := err.(*exec.ExitError); ok {
				code := exitErr.ExitCode()
				records[index].ExitCode = &code
			}
		}
		_ = m.save(records)
		return
	}
}

func (m Manager) List() []Record {
	contents, err := os.ReadFile(filepath.Join(m.layout.State, "processes.json"))
	if err != nil {
		return nil
	}
	var records []Record
	if json.Unmarshal(contents, &records) != nil {
		return nil
	}
	return records
}

// Reconcile refreshes persisted records whose process state may have changed
// while LDR was not running. It does not terminate anything; orphan cleanup is
// intentionally a separate operation.
func (m Manager) Reconcile() []Record {
	records := m.List()
	changed := false
	for index := range records {
		if records[index].Status != "RUNNING" && records[index].Status != "STOPPING" {
			continue
		}
		if processAlive(records[index].PID, records[index].PGID) {
			continue
		}
		finished := time.Now().UTC()
		records[index].FinishedAt = &finished
		records[index].Status = "ORPHANED"
		changed = true
	}
	if changed {
		_ = m.save(records)
	}
	return records
}

func (m Manager) Stop(id string) (Record, error) {
	records := m.List()
	for index := range records {
		if records[index].ID != id {
			continue
		}
		if records[index].Status != "RUNNING" && records[index].Status != "STOPPING" {
			return records[index], nil
		}
		records[index].Status = "STOPPING"
		if err := m.save(records); err != nil {
			return Record{}, err
		}
		if err := terminateProcess(records[index].PID, records[index].PGID, false); err != nil && !errors.Is(err, os.ErrProcessDone) {
			return Record{}, err
		}
		deadline := time.Now().Add(5 * time.Second)
		for processAlive(records[index].PID, records[index].PGID) && time.Now().Before(deadline) {
			time.Sleep(50 * time.Millisecond)
		}
		if processAlive(records[index].PID, records[index].PGID) {
			if err := terminateProcess(records[index].PID, records[index].PGID, true); err != nil && !errors.Is(err, os.ErrProcessDone) {
				return Record{}, err
			}
		}
		records[index].Status = "STOPPED"
		if err := m.save(records); err != nil {
			return Record{}, err
		}
		return records[index], nil
	}
	return Record{}, fmt.Errorf("process %q was not found", id)
}

func (m Manager) Restart(ctx context.Context, id string, task domain.Task, options execution.Options) (Record, error) {
	if _, err := m.Stop(id); err != nil {
		return Record{}, err
	}
	return m.Start(ctx, task, options)
}

func (m Manager) Logs(id string) ([]byte, error) {
	for _, record := range m.List() {
		if record.ID == id {
			contents, err := os.ReadFile(record.LogPath)
			if err != nil {
				return nil, err
			}
			const limit = 64 * 1024
			if len(contents) > limit {
				return append([]byte("... truncated ...\n"), contents[len(contents)-limit:]...), nil
			}
			return contents, nil
		}
	}
	return nil, fmt.Errorf("process %q was not found", id)
}

func (m Manager) save(records []Record) error {
	return state.WriteJSON(filepath.Join(m.layout.State, "processes.json"), records)
}

func safeID(value string) string {
	return strings.NewReplacer("/", "-", "\\", "-", ":", "-", " ", "-").Replace(value)
}
