// Package agentguidance manages a shared LDR runtime guide and links to it in
// global AI-agent instruction files. Repository-local instruction files are
// intentionally out of scope.
package agentguidance

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
)

const (
	GuideMarker = "<!-- ldr:runtime-guide:v1 -->"
	LinkMarker  = "<!-- ldr:runtime-guide-link:v1 -->"
)

const guideBlock = `

` + GuideMarker + `
# LDR Runtime Guide

Use this guide to discover the runtime version a project needs, locate LDR-managed runtimes, or install a missing version.

## Runtime store locations

LDR keeps shared runtimes outside project directories, in a per-user store:

| Platform | Default runtime root |
| --- | --- |
| macOS | ` + "`~/Library/Application Support/local-dev-runner/runtimes`" + ` |
| Linux | ` + "`$XDG_DATA_HOME/local-dev-runner/runtimes`" + `, or ` + "`~/.local/share/local-dev-runner/runtimes`" + ` when XDG_DATA_HOME is unset |
| Windows | ` + "`%LOCALAPPDATA%\\local-dev-runner\\runtimes`" + ` |

Set ` + "`LDR_RUNTIME_HOME`" + ` to override the root. Runtime families use stable major-version paths: ` + "`java/<major>`" + `, ` + "`node/<major>`" + `, and ` + "`go/1.<minor>`" + `.

## Find and select versions

- Search releases with ` + "`ldr runtime search java [query]`" + `, ` + "`ldr runtime search node [query]`" + `, or ` + "`ldr runtime search go [query]`" + `.
- List installed versions with ` + "`ldr runtime list`" + `.
- Select an installed version for shell links with ` + "`ldr runtime use java 21`" + ` (replace the family and version as needed).
- Project configuration can declare versions in ` + "`.ldr/project.toml`" + ` under ` + "`[runtimes]`" + `; project tasks resolve their declared or default runtime from LDR's store.

## Install a missing version

If the required version is not installed, use ` + "`ldr install java 21`" + `, ` + "`ldr install node 24`" + `, or ` + "`ldr install go 1.27`" + ` with the desired version. Check ` + "`ldr install --help`" + ` for LTS and release-search options. Runtime installation currently supports macOS.

Full instructions: [LDR README](https://github.com/swsw1005/local-dev-launcher/blob/main/README.md#runtime-store) and [runtime store details](https://github.com/swsw1005/local-dev-launcher/blob/main/docs/runtime-store.md).
<!-- /ldr:runtime-guide -->
`

const linkBlock = `

` + LinkMarker + `
For LDR runtime discovery and locations, read the [LDR runtime guide](../.ldr/ldr_runtime_guide.md) (` + "`~/.ldr/ldr_runtime_guide.md`" + `).
<!-- /ldr:runtime-guide-link -->
`

type File struct {
	Name        string
	Path        string
	Marker      string
	Block       string
	Exists      bool
	HasGuidance bool
	Added       bool
	Err         error
}

// GlobalFiles returns the guide and only the current user's global agent files.
func GlobalFiles() ([]File, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return nil, fmt.Errorf("find user home: %w", err)
	}
	files := []File{
		{Name: "LDR runtime guide", Path: filepath.Join(home, ".ldr", "ldr_runtime_guide.md"), Marker: GuideMarker, Block: guideBlock},
		{Name: "Codex global AGENTS.md link", Path: filepath.Join(home, ".codex", "AGENTS.md"), Marker: LinkMarker, Block: linkBlock},
		{Name: "Claude Code global CLAUDE.md link", Path: filepath.Join(home, ".claude", "CLAUDE.md"), Marker: LinkMarker, Block: linkBlock},
	}
	for i := range files {
		inspect(&files[i])
	}
	return files, nil
}

// AppendMissing creates the shared guide and appends links only when their
// unique markers are absent. Existing file contents are never rewritten.
func AppendMissing() ([]File, error) {
	files, err := GlobalFiles()
	if err != nil {
		return nil, err
	}
	// Write the guide first so the instruction links never point to a guide
	// that this operation has not yet created.
	appendMissing(&files[0])
	if files[0].Err != nil {
		for i := 1; i < len(files); i++ {
			files[i].Err = fmt.Errorf("runtime guide was not created: %w", files[0].Err)
		}
		return files, nil
	}
	for i := 1; i < len(files); i++ {
		appendMissing(&files[i])
	}
	return files, nil
}

// DetectedAgentFiles returns instruction files for the detected agent
// environments. The guide itself is not checked during normal startup.
func DetectedAgentFiles() []File {
	home, err := os.UserHomeDir()
	if err != nil {
		return nil
	}
	files := make([]File, 0, 2)
	if anyEnvironmentSet("CODEX_HOME", "CODEX_THREAD_ID", "CODEX_SESSION_ID") {
		files = append(files, File{Name: "Codex global AGENTS.md link", Path: filepath.Join(home, ".codex", "AGENTS.md"), Marker: LinkMarker})
	}
	if anyEnvironmentSet("CLAUDECODE", "CLAUDE_CODE", "CLAUDE_PROJECT_DIR") {
		files = append(files, File{Name: "Claude Code global CLAUDE.md link", Path: filepath.Join(home, ".claude", "CLAUDE.md"), Marker: LinkMarker})
	}
	if len(files) > 0 {
		guide := File{Name: "LDR runtime guide", Path: filepath.Join(home, ".ldr", "ldr_runtime_guide.md"), Marker: GuideMarker}
		inspect(&guide)
		files = append(files, guide)
	}
	for i := range files {
		inspect(&files[i])
	}
	return files
}

func inspect(file *File) {
	content, err := os.ReadFile(file.Path)
	if err == nil {
		file.Exists = true
		file.HasGuidance = strings.Contains(string(content), file.Marker)
	} else if !os.IsNotExist(err) {
		file.Err = err
	}
}

func appendMissing(file *File) {
	if file.Err != nil || file.HasGuidance {
		return
	}
	if err := os.MkdirAll(filepath.Dir(file.Path), 0o755); err != nil {
		file.Err = err
		return
	}
	f, err := os.OpenFile(file.Path, os.O_CREATE|os.O_RDWR|os.O_APPEND, 0o644)
	if err != nil {
		file.Err = err
		return
	}
	content, readErr := io.ReadAll(f)
	if readErr == nil && strings.Contains(string(content), file.Marker) {
		file.Exists = true
		file.HasGuidance = true
	} else if readErr != nil {
		file.Err = readErr
	} else {
		_, file.Err = f.WriteString(file.Block)
		if file.Err == nil {
			file.Exists = true
			file.HasGuidance = true
			file.Added = true
		}
	}
	if closeErr := f.Close(); file.Err == nil && closeErr != nil {
		file.Err = closeErr
	}
}

func anyEnvironmentSet(names ...string) bool {
	for _, name := range names {
		if os.Getenv(name) != "" {
			return true
		}
	}
	return false
}
