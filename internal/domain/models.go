// Package domain contains models shared by every LDR interface.
package domain

// Task is a normalized runnable task. Discovery adapters will populate it in
// Milestone 2; execution and future TUI/MCP layers consume the same model.
type Task struct {
	ID          string   `json:"id"`
	Name        string   `json:"name"`
	Group       string   `json:"group,omitempty"`
	Description string   `json:"description,omitempty"`
	Adapter     string   `json:"adapter"`
	Module      string   `json:"module,omitempty"`
	ModulePath  string   `json:"modulePath,omitempty"`
	WorkingDir  string   `json:"workingDir"`
	Command     string   `json:"command"`
	Args        []string `json:"args"`
}

// Runtime records a resolved (or unresolved) project runtime.
type Runtime struct {
	Type       string `json:"type"`
	Required   string `json:"required,omitempty"`
	Resolved   string `json:"resolved,omitempty"`
	Executable string `json:"executable,omitempty"`
	Status     string `json:"status"`
}

// Profile is user-managed execution configuration. Its persisted TOML format
// will be introduced with profile support in Milestone 5.
type Profile struct {
	Name    string            `json:"name"`
	Extends string            `json:"extends"`
	Env     map[string]string `json:"env,omitempty"`
}
