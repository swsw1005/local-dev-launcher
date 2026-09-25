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
	GuideMarker    = "<!-- ldr:runtime-guide:v1 -->"
	GuideEndMarker = "<!-- /ldr:runtime-guide -->"
	LinkMarker     = "<!-- ldr:runtime-guide-link:v1 -->"
	LinkEndMarker  = "<!-- /ldr:runtime-guide-link -->"
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
` + GuideEndMarker + `
`

const linkBlock = `

` + LinkMarker + `
For LDR runtime discovery and locations, read the [LDR runtime guide](../.ldr/ldr_runtime_guide.md) (` + "`~/.ldr/ldr_runtime_guide.md`" + `).
` + LinkEndMarker + `
`

type File struct {
	Name        string
	Path        string
	Marker      string
	EndMarker   string
	Block       string
	Exists      bool
	HasGuidance bool
	Damaged     bool
	Repairable  bool
	Problem     string
	Added       bool
	Repaired    bool
	Err         error
}

// GlobalFiles returns the guide and only the current user's global agent files.
func GlobalFiles() ([]File, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return nil, fmt.Errorf("find user home: %w", err)
	}
	files := []File{
		{Name: "LDR runtime guide", Path: filepath.Join(home, ".ldr", "ldr_runtime_guide.md"), Marker: GuideMarker, EndMarker: GuideEndMarker, Block: guideBlock},
		{Name: "Codex global AGENTS.md link", Path: filepath.Join(home, ".codex", "AGENTS.md"), Marker: LinkMarker, EndMarker: LinkEndMarker, Block: linkBlock},
		{Name: "Claude Code global CLAUDE.md link", Path: filepath.Join(home, ".claude", "CLAUDE.md"), Marker: LinkMarker, EndMarker: LinkEndMarker, Block: linkBlock},
	}
	for i := range files {
		inspect(&files[i])
	}
	return files, nil
}

// Apply adds missing managed blocks. With force enabled, it also replaces
// damaged blocks whose start and end markers still identify a safe range.
func Apply(force bool) ([]File, error) {
	files, err := GlobalFiles()
	if err != nil {
		return nil, err
	}
	applyFile(&files[0], force)
	if files[0].Err != nil {
		return files, nil
	}
	if files[0].Problem != "" || files[0].Damaged {
		return files, nil
	}
	for i := 1; i < len(files); i++ {
		applyFile(&files[i], force)
	}
	return files, nil
}

// DetectedAgentFiles returns instruction files and the shared guide for the
// detected agent environments.
func DetectedAgentFiles() []File {
	home, err := os.UserHomeDir()
	if err != nil {
		return nil
	}
	files := make([]File, 0, 3)
	if anyEnvironmentSet("CODEX_HOME", "CODEX_THREAD_ID", "CODEX_SESSION_ID") {
		files = append(files, File{Name: "Codex global AGENTS.md link", Path: filepath.Join(home, ".codex", "AGENTS.md"), Marker: LinkMarker, EndMarker: LinkEndMarker, Block: linkBlock})
	}
	if anyEnvironmentSet("CLAUDECODE", "CLAUDE_CODE", "CLAUDE_PROJECT_DIR") {
		files = append(files, File{Name: "Claude Code global CLAUDE.md link", Path: filepath.Join(home, ".claude", "CLAUDE.md"), Marker: LinkMarker, EndMarker: LinkEndMarker, Block: linkBlock})
	}
	if len(files) > 0 {
		files = append(files, File{Name: "LDR runtime guide", Path: filepath.Join(home, ".ldr", "ldr_runtime_guide.md"), Marker: GuideMarker, EndMarker: GuideEndMarker, Block: guideBlock})
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
		inspectBlock(file, string(content))
	} else if !os.IsNotExist(err) {
		file.Err = err
	}
}

func inspectBlock(file *File, content string) {
	file.HasGuidance = false
	file.Damaged = false
	file.Repairable = false
	file.Problem = ""
	starts := strings.Count(content, file.Marker)
	ends := strings.Count(content, file.EndMarker)
	if starts == 0 && ends == 0 {
		return
	}
	file.HasGuidance = true
	file.Damaged = true
	if starts != 1 || ends != 1 {
		file.Problem = "managed markers are missing or duplicated; remove the malformed marker block manually"
		return
	}
	start := strings.Index(content, file.Marker)
	end := strings.Index(content, file.EndMarker)
	if end < start+len(file.Marker) {
		file.Problem = "managed markers are out of order; remove the malformed marker block manually"
		return
	}
	file.Repairable = true
	managed := content[start : end+len(file.EndMarker)]
	if strings.TrimSpace(managed) == strings.TrimSpace(file.Block) {
		file.Damaged = false
		file.HasGuidance = true
		file.Repairable = false
	}
}

func applyFile(file *File, force bool) {
	if file.Err != nil || file.Problem != "" {
		return
	}
	if file.Damaged {
		if force && file.Repairable {
			replaceManagedBlock(file)
		}
		return
	}
	if file.HasGuidance {
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
	if readErr != nil {
		file.Err = readErr
	} else {
		file.Exists = true
		inspectBlock(file, string(content))
		if file.Problem == "" && file.Damaged {
			if force && file.Repairable {
				file.Err = f.Close()
				if file.Err == nil {
					replaceManagedBlock(file)
				}
				return
			}
		} else if !file.HasGuidance {
			_, file.Err = f.WriteString(file.Block)
			if file.Err == nil {
				file.HasGuidance = true
				file.Added = true
			}
		}
	}
	if closeErr := f.Close(); file.Err == nil && closeErr != nil {
		file.Err = closeErr
	}
}

func replaceManagedBlock(file *File) {
	content, err := os.ReadFile(file.Path)
	if err != nil {
		file.Err = err
		return
	}
	inspectBlock(file, string(content))
	if !file.Damaged || !file.Repairable {
		return
	}
	start := strings.Index(string(content), file.Marker)
	end := strings.Index(string(content), file.EndMarker) + len(file.EndMarker)
	updated := string(content[:start]) + strings.TrimSpace(file.Block) + string(content[end:])
	info, err := os.Stat(file.Path)
	if err != nil {
		file.Err = err
		return
	}
	if err := os.WriteFile(file.Path, []byte(updated), info.Mode().Perm()); err != nil {
		file.Err = err
		return
	}
	file.HasGuidance = true
	file.Damaged = false
	file.Repairable = false
	file.Repaired = true
}

func anyEnvironmentSet(names ...string) bool {
	for _, name := range names {
		if os.Getenv(name) != "" {
			return true
		}
	}
	return false
}
