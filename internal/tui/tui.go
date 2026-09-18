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
	groupEntry
	taskEntry
)

// browserEntry is one selectable Finder-style item in the current pane.
type browserEntry struct {
	id, label, description string
	kind                   entryKind
	modulePath             string
	adapter                string
	group                  string
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
	group      string
	selectedID string
}

// State preserves the current Finder-style location between task executions.
// Its fields stay private so navigation representation can evolve freely.
type State struct {
	locations []location
	notice    string
}

// WithNotice displays the result of the previously run task after the TUI
// returns to the same selected command.
func (s State) WithNotice(notice string) State {
	s.notice = notice
	return s
}

// Selection is the command chosen in a pane together with the pane state to
// restore once that command exits.
type Selection struct {
	TaskID string
	State  State
}

type model struct {
	list     list.Model
	tasks    []domain.Task
	stack    []location
	selected string
	notice   string
}

func newModel(tasks []domain.Task) model {
	return newModelWithState(tasks, State{})
}

func newModelWithState(tasks []domain.Task, state State) model {
	stack := append([]location(nil), state.locations...)
	if len(stack) == 0 {
		stack = []location{{modulePath: "."}}
	}
	m := model{tasks: tasks, stack: stack, notice: state.notice}
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

func (m model) state() State {
	return State{locations: append([]location(nil), m.stack...)}
}

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
		if item.group != "" {
			parts = append(parts, item.group)
			continue
		}
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
		localTasks := m.tasksAt(current.modulePath, current.adapter)
		if current.group != "" {
			return taskEntries(tasksInGroup(localTasks, current.group))
		}
		if current.adapter == "gradle" {
			return gradleEntries(localTasks)
		}
		if current.adapter == "maven" {
			return favoriteTaskEntries(localTasks)
		}
		return taskEntries(localTasks)
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
	} else if len(localTasks) > 0 && localTasks[0].Adapter == "gradle" {
		entries = append(entries, gradleEntries(localTasks)...)
	} else if len(localTasks) > 0 && localTasks[0].Adapter == "maven" {
		entries = append(entries, favoriteTaskEntries(localTasks)...)
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

func gradleEntries(tasks []domain.Task) []list.Item {
	items := make([]list.Item, 0, len(tasks))
	favorites := make([]domain.Task, 0, len(tasks))
	for _, task := range tasks {
		if !task.Favorite {
			continue
		}
		favorites = append(favorites, task)
	}
	sort.SliceStable(favorites, func(i, j int) bool { return favoriteOrder(favorites[i]) < favoriteOrder(favorites[j]) })
	for _, favorite := range favorites {
		items = append(items, browserEntry{id: "task:" + favorite.ID, kind: taskEntry, label: "★ " + favorite.Name, description: commandDescription(favorite), task: &favorite})
	}
	for _, group := range gradleGroups(tasks) {
		items = append(items, browserEntry{
			id:          "group:gradle:" + group,
			kind:        groupEntry,
			label:       "▸ " + group,
			description: fmt.Sprintf("%d commands", len(tasksInGroup(tasks, group))),
			adapter:     "gradle",
			group:       group,
		})
	}
	return items
}

func favoriteOrder(task domain.Task) string {
	priority := map[string]string{"spring-boot:run": "0", "spring-boot:run (local)": "1", "bootRun": "2", "build": "3", "clean": "4", "test": "5"}
	if value, exists := priority[task.Name]; exists {
		return value
	}
	return "9" + task.Name
}

func favoriteTaskEntries(tasks []domain.Task) []list.Item {
	favorites := make([]domain.Task, 0, len(tasks))
	others := make([]domain.Task, 0, len(tasks))
	for _, task := range tasks {
		if task.Favorite {
			favorites = append(favorites, task)
		} else {
			others = append(others, task)
		}
	}
	sort.SliceStable(favorites, func(i, j int) bool { return favoriteOrder(favorites[i]) < favoriteOrder(favorites[j]) })
	sort.SliceStable(others, func(i, j int) bool { return others[i].Name < others[j].Name })
	items := make([]list.Item, 0, len(tasks))
	for _, task := range append(favorites, others...) {
		label := task.Name
		if task.Favorite {
			label = "★ " + label
		}
		items = append(items, browserEntry{id: "task:" + task.ID, kind: taskEntry, label: label, description: commandDescription(task), task: &task})
	}
	return items
}

func gradleGroups(tasks []domain.Task) []string {
	groups := map[string]bool{}
	for _, task := range tasks {
		group := task.Group
		if group == "" {
			group = "Other tasks"
		}
		groups[group] = true
	}
	return sortedKeys(groups)
}

func tasksInGroup(tasks []domain.Task, group string) []domain.Task {
	var matches []domain.Task
	for _, task := range tasks {
		taskGroup := task.Group
		if taskGroup == "" {
			taskGroup = "Other tasks"
		}
		if taskGroup == group {
			matches = append(matches, task)
		}
	}
	return matches
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
		height := size.Height - m.footerHeight()
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
					m.stack[len(m.stack)-1].selectedID = selected.id
					m.selected = selected.task.ID
					return m, tea.Quit
				}
				break
			}
			m.stack[len(m.stack)-1].selectedID = selected.id
			modulePath := selected.modulePath
			if modulePath == "" {
				modulePath = m.current().modulePath
			}
			m.stack = append(m.stack, location{modulePath: modulePath, adapter: selected.adapter, group: selected.group})
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
	footer := "Enter/→ open · ← back · Enter on a command runs · / search · q quit"
	if m.notice != "" {
		footer = m.notice + "\n" + footer
	}
	return m.list.View() + "\n" + footer
}

func (m model) footerHeight() int {
	if m.notice != "" {
		return 2
	}
	return 1
}

func Select(in io.Reader, out io.Writer, tasks []domain.Task) (string, error) {
	selection, err := SelectWithState(in, out, tasks, State{})
	return selection.TaskID, err
}

// SelectWithState opens the TUI at a previously selected command when state
// comes from an earlier task execution.
func SelectWithState(in io.Reader, out io.Writer, tasks []domain.Task, state State) (Selection, error) {
	program := tea.NewProgram(newModelWithState(tasks, state), tea.WithInput(in), tea.WithOutput(out), tea.WithAltScreen())
	result, err := program.Run()
	if err != nil {
		return Selection{}, err
	}
	m := result.(model)
	return Selection{TaskID: m.selected, State: m.state()}, nil
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
