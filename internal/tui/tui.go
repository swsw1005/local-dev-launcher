// Package tui provides the interactive client for LDR's shared task registry.
package tui

import (
	"fmt"
	"io"
	"path"
	"sort"
	"strings"

	"github.com/charmbracelet/bubbles/list"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/swsw1005/local-dev-launcher/internal/domain"
)

type entryKind int

const (
	moduleEntry entryKind = iota
	adapterEntry
	taskEntry
)

// browserEntry is one selectable Finder-style item in the current pane.
type browserEntry struct {
	id, label, description string
	kind                   entryKind
	modulePath             string
	adapter                string
	task                   *domain.Task
}

func (entry browserEntry) FilterValue() string {
	if entry.task == nil {
		return entry.label + " " + entry.description
	}
	return entry.task.ID + " " + entry.task.Name + " " + entry.task.ModulePath
}

func (entry browserEntry) Title() string {
	if entry.description == "" {
		return entry.label
	}
	return entry.label + "  · " + entry.description
}
func (browserEntry) Description() string { return "" }

type location struct {
	modulePath string
	adapter    string
	selectedID string
}

type model struct {
	list     list.Model
	tasks    []domain.Task
	stack    []location
	selected string
}

func newModel(tasks []domain.Task) model {
	m := model{tasks: tasks, stack: []location{{modulePath: "."}}}
	m.list = newList(m.entries())
	m.updateTitle()
	return m
}

func newList(items []list.Item) list.Model {
	delegate := list.NewDefaultDelegate()
	delegate.ShowDescription = false
	delegate.SetHeight(1)
	delegate.SetSpacing(0)
	l := list.New(items, delegate, 0, 0)
	l.Styles.Title = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("86"))
	l.SetFilteringEnabled(true)
	l.SetShowStatusBar(false)
	l.SetShowPagination(false)
	l.SetShowHelp(false)
	l.KeyMap.Quit.SetKeys("q", "ctrl+c")
	l.KeyMap.ForceQuit.SetKeys("q", "ctrl+c")
	return l
}

func (m model) current() location { return m.stack[len(m.stack)-1] }

func (m *model) refreshList() {
	items := m.entries()
	_ = m.list.SetItems(items)
	m.list.ResetSelected()
	if selectedID := m.current().selectedID; selectedID != "" {
		for index, item := range items {
			if entry, ok := item.(browserEntry); ok && entry.id == selectedID {
				m.list.Select(index)
				break
			}
		}
	}
	m.updateTitle()
}

func (m *model) updateTitle() {
	m.list.Title = "Local Dev Runner  ›  " + m.breadcrumb()
}

func (m model) breadcrumb() string {
	parts := []string{"project root"}
	for _, item := range m.stack[1:] {
		if item.adapter != "" {
			parts = append(parts, strings.ToUpper(item.adapter))
			continue
		}
		parts = append(parts, path.Base(item.modulePath))
	}
	return strings.Join(parts, " › ")
}

func (m model) entries() []list.Item {
	current := m.current()
	if current.adapter != "" {
		return taskEntries(m.tasksAt(current.modulePath, current.adapter))
	}

	children := map[string]string{}
	for _, task := range m.tasks {
		modulePath := normalizedModulePath(task)
		relative, ok := relativeChild(current.modulePath, modulePath)
		if !ok || relative == "" {
			continue
		}
		segment := strings.Split(relative, "/")[0]
		target := segment
		if current.modulePath != "." {
			target = path.Join(current.modulePath, segment)
		}
		children[segment] = target
	}

	entries := make([]list.Item, 0, len(children)+len(m.tasks))
	for _, name := range sortedKeys(children) {
		entries = append(entries, browserEntry{
			id:          "module:" + children[name],
			kind:        moduleEntry,
			label:       "▸ " + name,
			description: children[name],
			modulePath:  children[name],
		})
	}

	localTasks := m.tasksAt(current.modulePath, "")
	if current.modulePath == "." || adapterCount(localTasks) > 1 {
		for _, adapter := range adapters(localTasks) {
			entries = append(entries, browserEntry{
				id:          "adapter:" + current.modulePath + ":" + adapter,
				kind:        adapterEntry,
				label:       "▸ " + strings.ToUpper(adapter),
				description: fmt.Sprintf("%d commands", len(m.tasksAt(current.modulePath, adapter))),
				modulePath:  current.modulePath,
				adapter:     adapter,
			})
		}
	} else {
		entries = append(entries, taskEntries(localTasks)...)
	}
	return entries
}

func (m model) tasksAt(modulePath, adapter string) []domain.Task {
	var matches []domain.Task
	for _, task := range m.tasks {
		if normalizedModulePath(task) != modulePath || (adapter != "" && task.Adapter != adapter) {
			continue
		}
		matches = append(matches, task)
	}
	sort.Slice(matches, func(i, j int) bool {
		if matches[i].Adapter != matches[j].Adapter {
			return matches[i].Adapter < matches[j].Adapter
		}
		return matches[i].Name < matches[j].Name
	})
	return matches
}

func taskEntries(tasks []domain.Task) []list.Item {
	items := make([]list.Item, 0, len(tasks))
	for index := range tasks {
		task := tasks[index]
		items = append(items, browserEntry{id: "task:" + task.ID, kind: taskEntry, label: task.Name, description: commandDescription(task), task: &task})
	}
	return items
}

func adapterCount(tasks []domain.Task) int { return len(adapters(tasks)) }

func adapters(tasks []domain.Task) []string {
	values := map[string]bool{}
	for _, task := range tasks {
		values[task.Adapter] = true
	}
	return sortedKeys(values)
}

func normalizedModulePath(task domain.Task) string {
	if task.ModulePath == "" || task.ModulePath == "." {
		return "."
	}
	return path.Clean(task.ModulePath)
}

func relativeChild(parent, child string) (string, bool) {
	if parent == "." {
		if child == "." {
			return "", true
		}
		return child, true
	}
	if child == parent {
		return "", true
	}
	prefix := parent + "/"
	if !strings.HasPrefix(child, prefix) {
		return "", false
	}
	return strings.TrimPrefix(child, prefix), true
}

func (m model) Init() tea.Cmd { return nil }

func (m model) Update(message tea.Msg) (tea.Model, tea.Cmd) {
	if size, ok := message.(tea.WindowSizeMsg); ok {
		height := size.Height - 1
		if height < 1 {
			height = 1
		}
		m.list.SetSize(size.Width, height)
	}
	if key, ok := message.(tea.KeyMsg); ok {
		switch key.String() {
		case "enter", "right":
			selected, ok := m.list.SelectedItem().(browserEntry)
			if !ok {
				break
			}
			if selected.kind == taskEntry {
				if key.String() == "enter" {
					m.selected = selected.task.ID
					return m, tea.Quit
				}
				break
			}
			m.stack[len(m.stack)-1].selectedID = selected.id
			m.stack = append(m.stack, location{modulePath: selected.modulePath, adapter: selected.adapter})
			m.refreshList()
			return m, nil
		case "left", "backspace", "esc":
			if len(m.stack) > 1 {
				m.stack = m.stack[:len(m.stack)-1]
				m.refreshList()
				return m, nil
			}
		case "q", "ctrl+c":
			return m, tea.Quit
		}
	}
	var command tea.Cmd
	m.list, command = m.list.Update(message)
	return m, command
}

func (m model) View() string {
	return m.list.View() + "\nEnter/→ open · ← back · Enter on a command runs · / search · q quit"
}

func Select(in io.Reader, out io.Writer, tasks []domain.Task) (string, error) {
	program := tea.NewProgram(newModel(tasks), tea.WithInput(in), tea.WithOutput(out), tea.WithAltScreen())
	result, err := program.Run()
	if err != nil {
		return "", err
	}
	return result.(model).selected, nil
}

func commandDescription(task domain.Task) string {
	command := task.Command
	for _, arg := range task.Args {
		command += " " + arg
	}
	return command
}

func sortedKeys[T any](values map[string]T) []string {
	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}
