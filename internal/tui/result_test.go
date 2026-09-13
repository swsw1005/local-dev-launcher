package tui

import (
	"os"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/swsw1005/local-dev-launcher/internal/domain"
)

func TestReadResultTailKeepsLastOutput(t *testing.T) {
	logPath := t.TempDir() + "/task.log"
	contents := strings.Repeat("a", maxResultLogBytes) + "last line\n"
	if err := os.WriteFile(logPath, []byte(contents), 0o644); err != nil {
		t.Fatal(err)
	}
	got, err := readResultTail(logPath)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(got), "last line\n") || !strings.Contains(string(got), "Earlier output omitted") {
		t.Fatalf("tail did not retain final output: %q", string(got[len(got)-64:]))
	}
}

func TestResultViewShowsStatusAndRerunHint(t *testing.T) {
	m := newResultModel(RunResult{Task: domain.Task{Name: "integrationTest"}, LogPath: "/tmp/run.log"}, "final output\n")
	view := m.View()
	for _, want := range []string{"Finished successfully", "Enter rerun", "integrationTest"} {
		if !strings.Contains(view, want) {
			t.Fatalf("view = %q, missing %q", view, want)
		}
	}
}

func TestResultEnterSelectsRerun(t *testing.T) {
	m := newResultModel(RunResult{Task: domain.Task{Name: "build"}}, "final output\n")
	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if got := updated.(resultModel).action; got != ResultRerun {
		t.Fatalf("action = %v, want rerun", got)
	}
}
