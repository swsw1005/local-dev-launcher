// Package process persists minimal metadata for LDR-managed background tasks.
package process

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/swsw1005/local-dev-launcher/internal/domain"
	"github.com/swsw1005/local-dev-launcher/internal/execution"
	"github.com/swsw1005/local-dev-launcher/internal/state"
)

type Record struct {
	ID        string    `json:"id"`
	TaskID    string    `json:"taskId"`
	PID       int       `json:"pid"`
	StartedAt time.Time `json:"startedAt"`
	LogPath   string    `json:"logPath"`
	Status    string    `json:"status"`
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
	if err := command.Start(); err != nil {
		logFile.Close()
		return Record{}, fmt.Errorf("start %s: %w", task.ID, err)
	}
	if err := logFile.Close(); err != nil {
		return Record{}, err
	}
	record := Record{ID: id, TaskID: task.ID, PID: command.Process.Pid, StartedAt: time.Now().UTC(), LogPath: logPath, Status: "RUNNING"}
	if err := m.save(append(m.List(), record)); err != nil {
		return Record{}, err
	}
	if err := command.Process.Release(); err != nil {
		return Record{}, err
	}
	return record, nil
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

func (m Manager) Stop(id string) (Record, error) {
	records := m.List()
	for index := range records {
		if records[index].ID != id {
			continue
		}
		if records[index].Status != "RUNNING" {
			return records[index], nil
		}
		process, err := os.FindProcess(records[index].PID)
		if err != nil {
			return Record{}, err
		}
		if err := process.Kill(); err != nil && !errors.Is(err, os.ErrProcessDone) {
			return Record{}, err
		}
		records[index].Status = "STOPPED"
		if err := m.save(records); err != nil {
			return Record{}, err
		}
		return records[index], nil
	}
	return Record{}, fmt.Errorf("process %q was not found", id)
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
	contents, err := json.MarshalIndent(records, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(m.layout.State, "processes.json"), append(contents, '\n'), 0o644)
}

func safeID(value string) string {
	return strings.NewReplacer("/", "-", "\\", "-", ":", "-", " ", "-").Replace(value)
}
