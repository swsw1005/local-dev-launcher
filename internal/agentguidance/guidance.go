// Package agentguidance manages the LDR runtime note in global AI-agent
// instruction files. Repository-local instruction files are intentionally
// out of scope.
package agentguidance

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
)

const Marker = "<!-- ldr:runtime-guidance:v1 -->"

const guidance = `

` + Marker + `
## LDR managed runtimes

LDR stores managed Java, Node.js, and Go runtimes in the per-user runtime store
(on macOS: ` + "`~/Library/Application Support/local-dev-runner/runtimes`" + `; other platform paths and ` + "`LDR_RUNTIME_HOME`" + ` overrides are documented below).
If a project needs a Java, Node.js, or Go version that is not available, install it with ` + "`ldr install <java|node|go> <version>`" + `. Use ` + "`ldr runtime list`" + ` and ` + "`ldr runtime use <family> <version>`" + ` to inspect and select installed versions.
See [LDR runtime and installation instructions](https://github.com/swsw1005/local-dev-launcher/blob/main/README.md#runtime-store) and [runtime store details](https://github.com/swsw1005/local-dev-launcher/blob/main/docs/runtime-store.md).
<!-- /ldr:runtime-guidance -->
`

type File struct {
	Name        string
	Path        string
	Exists      bool
	HasGuidance bool
	Added       bool
	Err         error
}

// GlobalFiles returns only the current user's Codex and Claude global files.
func GlobalFiles() ([]File, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return nil, fmt.Errorf("find user home: %w", err)
	}
	paths := []File{
		{Name: "Codex global AGENTS.md", Path: filepath.Join(home, ".codex", "AGENTS.md")},
		{Name: "Claude global CLAUDE.md", Path: filepath.Join(home, ".claude", "CLAUDE.md")},
	}
	for i := range paths {
		content, readErr := os.ReadFile(paths[i].Path)
		if readErr == nil {
			paths[i].Exists = true
			paths[i].HasGuidance = strings.Contains(string(content), Marker)
		} else if !os.IsNotExist(readErr) {
			paths[i].Err = readErr
		}
	}
	return paths, nil
}

// AppendMissing adds the managed block only when its unique marker is absent.
// Opening in append mode preserves every existing byte in the instruction file.
func AppendMissing() ([]File, error) {
	files, err := GlobalFiles()
	if err != nil {
		return nil, err
	}
	for i := range files {
		if files[i].Err != nil || files[i].HasGuidance {
			continue
		}
		if err := os.MkdirAll(filepath.Dir(files[i].Path), 0o755); err != nil {
			files[i].Err = err
			continue
		}
		f, err := os.OpenFile(files[i].Path, os.O_CREATE|os.O_RDWR|os.O_APPEND, 0o644)
		if err != nil {
			files[i].Err = err
			continue
		}
		content, readErr := io.ReadAll(f)
		if readErr == nil && strings.Contains(string(content), Marker) {
			files[i].Exists = true
			files[i].HasGuidance = true
		} else if readErr != nil {
			files[i].Err = readErr
		} else {
			_, files[i].Err = f.WriteString(guidance)
			if files[i].Err == nil {
				files[i].Exists = true
				files[i].HasGuidance = true
				files[i].Added = true
			}
		}
		if closeErr := f.Close(); files[i].Err == nil && closeErr != nil {
			files[i].Err = closeErr
		}
	}
	return files, nil
}

// DetectedAgentFiles returns global instruction files for detected agent
// environments.
func DetectedAgentFiles() []File {
	files := make([]File, 0, 2)
	home, err := os.UserHomeDir()
	if err != nil {
		return files
	}
	if anyEnvironmentSet("CODEX_HOME", "CODEX_THREAD_ID", "CODEX_SESSION_ID") {
		files = append(files, File{Name: "Codex global AGENTS.md", Path: filepath.Join(home, ".codex", "AGENTS.md")})
	}
	if anyEnvironmentSet("CLAUDECODE", "CLAUDE_CODE", "CLAUDE_PROJECT_DIR") {
		files = append(files, File{Name: "Claude global CLAUDE.md", Path: filepath.Join(home, ".claude", "CLAUDE.md")})
	}
	for i := range files {
		content, readErr := os.ReadFile(files[i].Path)
		if readErr == nil {
			files[i].Exists = true
			files[i].HasGuidance = strings.Contains(string(content), Marker)
		} else if !os.IsNotExist(readErr) {
			files[i].Err = readErr
		}
	}
	return files
}

func anyEnvironmentSet(names ...string) bool {
	for _, name := range names {
		if os.Getenv(name) != "" {
			return true
		}
	}
	return false
}
