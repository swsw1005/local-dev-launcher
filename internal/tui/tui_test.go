package tui

import (
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/swsw1005/local-dev-launcher/internal/domain"
)

func TestRootListsModulesAndRootAdapter(t *testing.T) {
	tasks := []domain.Task{
		{ID: "gradle.root.clean", Name: "clean", Adapter: "gradle", ModulePath: ".", Command: "./gradlew"},
		{ID: "gradle.api.clean", Name: "clean", Adapter: "gradle", ModulePath: "homeops-admin-api", Command: "./gradlew"},
		{ID: "node.web.build", Name: "build", Adapter: "node", ModulePath: "homeops-admin-fe", Command: "npm"},
	}
	m := newModel(tasks)
	items := m.entries()
	if len(items) != 3 {
		t.Fatalf("root items = %d, want two modules and root Gradle", len(items))
	}
	if got := items[0].(browserEntry).label; got != "▸ homeops-admin-api" {
		t.Fatalf("first root item = %q", got)
	}
	if got := items[2].(browserEntry).label; got != "▸ GRADLE" {
		t.Fatalf("root adapter = %q", got)
	}
}

func TestGradleModuleShowsFavoritesThenTaskCategories(t *testing.T) {
	tasks := []domain.Task{
		{ID: "gradle.api.clean", Name: "clean", Favorite: true, Group: "Build tasks", Adapter: "gradle", ModulePath: "homeops-admin-api", Command: "./gradlew", Args: []string{":homeops-admin-api:clean"}},
		{ID: "gradle.api.integrationTest", Name: "integrationTest", Group: "Verification tasks", Adapter: "gradle", ModulePath: "homeops-admin-api", Command: "./gradlew", Args: []string{":homeops-admin-api:integrationTest"}},
	}
	m := newModel(tasks)
	m.stack = append(m.stack, location{modulePath: "homeops-admin-api"})
	items := m.entries()
	if len(items) != 3 {
		t.Fatalf("module items = %d, want favorite and two categories", len(items))
	}
	entry := items[0].(browserEntry)
	if entry.kind != taskEntry || entry.label != "★ clean" || entry.description != "./gradlew :homeops-admin-api:clean" {
		t.Fatalf("favorite command = %#v", entry)
	}
	category := items[2].(browserEntry)
	if category.kind != groupEntry || category.label != "▸ Verification tasks" {
		t.Fatalf("category = %#v", category)
	}
}

func TestBackRestoresTheItemThatOpenedTheCurrentPane(t *testing.T) {
	tasks := []domain.Task{
		{ID: "gradle.api.clean", Name: "clean", Adapter: "gradle", ModulePath: "homeops-admin-api", Command: "./gradlew"},
		{ID: "gradle.web.clean", Name: "clean", Adapter: "gradle", ModulePath: "homeops-admin-fe", Command: "./gradlew"},
	}
	m := newModel(tasks)
	m.list.Select(1)
	selected := m.list.SelectedItem().(browserEntry)
	m.stack[len(m.stack)-1].selectedID = selected.id
	m.stack = append(m.stack, location{modulePath: selected.modulePath})
	m.refreshList()
	m.stack = m.stack[:len(m.stack)-1]
	m.refreshList()
	if got := m.list.SelectedItem().(browserEntry).id; got != selected.id {
		t.Fatalf("restored selection = %q, want %q", got, selected.id)
	}
}

func TestWindowSizeSetsListDimensions(t *testing.T) {
	initial := newModel([]domain.Task{{ID: "gradle.api.bootRun", Name: "bootRun", ModulePath: "api"}})
	updated, _ := initial.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
	got := updated.(model)
	if got.list.Width() != 120 || got.list.Height() != 39 {
		t.Fatalf("list size = %dx%d, want 120x39", got.list.Width(), got.list.Height())
	}
}
