package tui

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/swsw1005/local-dev-launcher/internal/domain"
)

const maxResultLogBytes = 512 * 1024

// RunResult describes a completed foreground task and the log retained for it.
type RunResult struct {
	Task    domain.Task
	LogPath string
	Err     error
}

// ResultAction is the user's next action after inspecting a completed task.
type ResultAction int

const (
	ResultQuit ResultAction = iota
	ResultBack
	ResultRerun
)

// ShowRunResult opens the final-output view with its viewport positioned at the
// end of the captured log. The full log remains available on disk; the view
// displays its final 512 KiB so a runaway process cannot overwhelm the TUI.
func ShowRunResult(in io.Reader, out io.Writer, result RunResult) (ResultAction, error) {
	contents, err := readResultTail(result.LogPath)
	if err != nil {
		return ResultQuit, fmt.Errorf("read task log: %w", err)
	}
	program := tea.NewProgram(newResultModel(result, string(contents)), tea.WithInput(in), tea.WithOutput(out), tea.WithAltScreen())
	completed, err := program.Run()
	if err != nil {
		return ResultQuit, err
	}
	return completed.(resultModel).action, nil
}

func readResultTail(logPath string) ([]byte, error) {
	info, err := os.Stat(logPath)
	if err != nil {
		return nil, err
	}
	file, err := os.Open(logPath)
	if err != nil {
		return nil, err
	}
	defer file.Close()
	offset := int64(0)
	truncated := info.Size() > maxResultLogBytes
	if truncated {
		offset = info.Size() - maxResultLogBytes
	}
	if _, err := file.Seek(offset, io.SeekStart); err != nil {
		return nil, err
	}
	contents, err := io.ReadAll(file)
	if err != nil {
		return nil, err
	}
	if truncated {
		return append([]byte("[LDR] Earlier output omitted from this view. Full log: "+logPath+"\n\n"), contents...), nil
	}
	return contents, nil
}

type resultModel struct {
	result   RunResult
	contents string
	viewport viewport.Model
	ready    bool
	action   ResultAction
}

func newResultModel(result RunResult, contents string) resultModel {
	return resultModel{result: result, contents: contents}
}

func (m resultModel) Init() tea.Cmd { return nil }

func (m resultModel) Update(message tea.Msg) (tea.Model, tea.Cmd) {
	if size, ok := message.(tea.WindowSizeMsg); ok {
		height := size.Height - 5
		if height < 1 {
			height = 1
		}
		if !m.ready {
			m.viewport = viewport.New(size.Width, height)
			m.viewport.SetContent(m.contents)
			m.viewport.GotoBottom()
			m.ready = true
		} else {
			m.viewport.Width = size.Width
			m.viewport.Height = height
		}
		return m, nil
	}
	if key, ok := message.(tea.KeyMsg); ok {
		switch key.String() {
		case "enter":
			m.action = ResultRerun
			return m, tea.Quit
		case "left", "backspace", "esc":
			m.action = ResultBack
			return m, tea.Quit
		case "q", "ctrl+c":
			m.action = ResultQuit
			return m, tea.Quit
		}
	}
	var command tea.Cmd
	m.viewport, command = m.viewport.Update(message)
	return m, command
}

func (m resultModel) View() string {
	status := "Finished successfully"
	if m.result.Err != nil {
		status = "Stopped or failed: " + m.result.Err.Error()
	}
	header := lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("86")).Render("Local Dev Runner  ›  Last run  ›  " + m.result.Task.Name)
	logPath := "Log: " + filepath.ToSlash(m.result.LogPath)
	footer := "↑/↓ scroll · Enter rerun · ← task list · q quit"
	if !m.ready {
		return strings.Join([]string{header, status, logPath, footer}, "\n")
	}
	return strings.Join([]string{header, status, logPath, m.viewport.View(), footer}, "\n")
}
