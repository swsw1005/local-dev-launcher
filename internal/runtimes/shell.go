package runtimes

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

const shellSourceLine = `[ -f "$HOME/.shell_paths" ] && . "$HOME/.shell_paths"`

const shellManagedStart = "# Local Dev Runner shared paths."
const shellManagedEnd = "# End Local Dev Runner shared paths."

const shellManagedBlock = `
# Local Dev Runner shared paths. Keep this block so LDR-managed runtimes are
# available to Bash, Zsh, IDE terminals, and non-interactive tools.
ldr_dedupe_path() {
  PATH="$(printf '%s' "$PATH" | awk -v RS=: '!seen[$0]++ && length($0) { result = result ? result ":" $0 : $0 } END { print result }')"
}
ldr_prepend_path() {
  case ":$PATH:" in
    *":$1:"*) ;;
    *) PATH="$1:$PATH" ;;
  esac
}
ldr_prepend_path "$HOME/bin"
ldr_prepend_path "/opt/homebrew/bin"
ldr_prepend_path "$HOME/.local/bin"
ldr_dedupe_path
export PATH
unset -f ldr_prepend_path ldr_dedupe_path 2>/dev/null || true

LDR_SHELL_HOME="$HOME/Library/Application Support/local-dev-runner/shell"
if [ -f "$LDR_SHELL_HOME/banner.sh" ]; then
  . "$LDR_SHELL_HOME/banner.sh"
  case "$-" in
    *i*) command -v ldr_show_banner >/dev/null 2>&1 && ldr_show_banner ;;
  esac
fi
# End Local Dev Runner shared paths.
`

type ShellInitResult struct {
	ShellPaths string
	ShellHome  string
}

// InitShell installs a shared shell-path file and makes Bash and Zsh source
// it. Existing shell-path content is retained verbatim before LDR's block.
// banner.sh is deliberately optional: users may add it under ShellHome later.
func InitShell() (ShellInitResult, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return ShellInitResult{}, err
	}
	store, err := StoreRoot()
	if err != nil {
		return ShellInitResult{}, err
	}
	shellHome := filepath.Dir(store) + string(filepath.Separator) + "shell"
	if err := os.MkdirAll(shellHome, 0o755); err != nil {
		return ShellInitResult{}, err
	}
	paths := filepath.Join(home, ".shell_paths")
	if err := ensureShellPaths(paths); err != nil {
		return ShellInitResult{}, err
	}
	for _, name := range []string{".bashrc", ".zshrc"} {
		if err := ensureShellSource(filepath.Join(home, name)); err != nil {
			return ShellInitResult{}, err
		}
	}
	return ShellInitResult{ShellPaths: paths, ShellHome: shellHome}, nil
}

func ensureShellPaths(path string) error {
	contents, err := os.ReadFile(path)
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	if start := strings.Index(string(contents), shellManagedStart); start >= 0 {
		end := strings.Index(string(contents[start:]), shellManagedEnd)
		if end >= 0 {
			end += start + len(shellManagedEnd)
			prefix := strings.TrimRight(string(contents[:start]), "\n")
			suffix := strings.TrimLeft(string(contents[end:]), "\n")
			contents = append([]byte(prefix), []byte(shellManagedBlock)...)
			if suffix != "" {
				contents = append(contents, '\n')
				contents = append(contents, []byte(suffix)...)
			}
			return os.WriteFile(path, contents, 0o644)
		}
		// v0.5.0-dev's initial block was always appended by LDR, so replacing
		// it to EOF preserves the user-managed prefix while upgrading safely.
		contents = append([]byte(strings.TrimRight(string(contents[:start]), "\n")), []byte(shellManagedBlock)...)
		return os.WriteFile(path, contents, 0o644)
	}
	if info, err := os.Lstat(path); err == nil && info.Mode()&os.ModeSymlink != 0 {
		contents, err = os.ReadFile(path)
		if err != nil {
			return err
		}
		if err := os.Remove(path); err != nil {
			return err
		}
	}
	if len(contents) > 0 && contents[len(contents)-1] != '\n' {
		contents = append(contents, '\n')
	}
	contents = append(contents, []byte(shellManagedBlock)...)
	return os.WriteFile(path, contents, 0o644)
}

func ensureShellSource(path string) error {
	contents, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		contents = nil
	} else if err != nil {
		return err
	}
	lines := strings.Split(string(contents), "\n")
	filtered := make([]string, 0, len(lines))
	for _, line := range lines {
		if strings.TrimSpace(line) != shellSourceLine {
			filtered = append(filtered, line)
		}
	}
	contents = []byte(strings.TrimRight(strings.Join(filtered, "\n"), "\n"))
	if len(contents) > 0 && contents[len(contents)-1] != '\n' {
		contents = append(contents, '\n')
	}
	contents = append(contents, '\n')
	contents = append(contents, []byte(shellSourceLine)...)
	contents = append(contents, '\n')
	if err := os.WriteFile(path, contents, 0o644); err != nil {
		return fmt.Errorf("update %s: %w", path, err)
	}
	return nil
}
