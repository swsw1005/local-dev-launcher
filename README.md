# Local Dev Runner (LDR) 0.7.0

Local Dev Runner is a terminal-first local development runner. It is designed
to discover project tasks and provide one execution model for developers,
coding agents, and a future TUI.

```text
Discover → Resolve → Run → Manage
```

## Status

The 0.7.0 CLI provides:

- `ldr init` to create isolated project-local state
- Git ignore detection and a clear warning for unignored `.ldr/` directories
- normalized task registry and SHA-256 discovery cache
- direct structured task execution with streamed output and exit-code propagation
- user profiles with environment and argument overrides
- profile `.env` files, variable interpolation, and masked display
- fuzzy task search, aliases, and recent-task history
- `ldr doctor` diagnostics and machine-readable process/task output
- explicit orphan-process discovery and cleanup
- optional `.ldr/project.toml` discovery configuration
- Cargo, Makefile, and Docker Compose task discovery
- user-level runtime-store paths
- Go runtime requirement parsing and resolution (`go.mod` / `go.work`)

Gradle, Maven, Node, and Go task discovery, the Finder-style TUI, and
long-running process management are available on macOS.

LDR can also download and update the Java, Node, and Go runtimes it resolves.
Run `ldr install --help` for the runtime installer commands.

Go installs can use `ldr install go latest`, an exact release such as
`ldr install go 1.26.3`, or `ldr install go --list 1.26` to search stable
releases published by Go.
Available releases can be searched with `ldr runtime search go`,
`ldr runtime search java 21`, or `ldr runtime search node 24`.

After installation, LDR activates the newest selected family through `~/bin`
links (`java`/`javac`, `node`/`npm`/`npx`/`pnpm`, or `go`/`gofmt`). Use `ldr runtime
list` to inspect installed families and `ldr runtime use java 21` to change the
shell-active version.

Use `ldr runtime remove java 21` to remove an installed runtime family. If it
is active, its `~/bin` links are removed too.

`ldr init-shell` creates or extends `~/.shell_paths` without discarding its
existing contents, then makes Bash and Zsh source it. The generated file keeps
`~/bin`, Homebrew, and local-bin paths consistent. If
`~/Library/Application Support/local-dev-runner/shell/banner.sh` exists, it is
sourced only by interactive shells; no banner is shown when it is absent.

## Install on macOS

Go is not required for normal installation. Download the matching prebuilt
binary from the [latest release](https://github.com/swsw1005/local-dev-launcher/releases/latest),
then extract and install it:

```bash
# Apple Silicon (M1/M2/M3/M4)
gh release download v0.7.0 --repo swsw1005/local-dev-launcher --pattern 'ldr_0.7.0_darwin_arm64.tar.gz'
tar -xzf ldr_0.7.0_darwin_arm64.tar.gz
install -m 0755 ldr "$HOME/bin/ldr"
```

For Intel Macs, replace `darwin_arm64` with `darwin_amd64`. The repository is
private, so `gh auth login` is required for the command above; downloading the
release asset in GitHub's browser UI works as well.

If `~/bin` is not on your PATH, add this to your shell configuration:

```bash
export PATH="$HOME/bin:$PATH"
```

## Build from source

LDR is written in Go. Only contributors who want to build it themselves need
Go installed:

```bash
go build -o ~/bin/ldr ./cmd/ldr
```

Then run it from any project directory:

```bash
ldr
ldr init
ldr list
ldr list --json
ldr list --search boot --json
ldr list --recent
ldr refresh
ldr run gradle.homeops-agent-api.bootRun
ldr start gradle.homeops-agent-api.bootRun
ldr ps
ldr tui
ldr profile clone gradle.homeops-agent-api.bootRun agent-api-local
ldr profile list
ldr --help
ldr --version
```

## Project-local state

On first use in a project, `ldr` (or `ldr init`) creates local state in the
detected project root without blocking. `ldr init --yes` remains accepted for
script compatibility. `ldr init` creates only local state in the detected
project root:

```text
.ldr/
├── cache/      # LDR-generated and disposable
├── profiles/   # user-managed; never overwritten by discovery
└── state/      # ephemeral process state
```

LDR never silently changes `.gitignore`. In a Git worktree it emits an
English/Korean warning when `.ldr/` is not ignored, then continues; add the
following entry yourself:

```gitignore
.ldr/
```

## Discover tasks

```bash
ldr list
ldr list --json
ldr refresh
```

LDR asks a Gradle wrapper for its configured `tasks --all` report, so plugin
and project-specific tasks such as `integrationTest` are discovered alongside
common Gradle tasks. Maven lifecycle, package.json scripts, and Go build/test
tasks are statically discovered. The normalized registry is cached under
`.ldr/cache/`; unchanged projects reuse it, while `ldr refresh` rebuilds it
and reruns the Gradle report when a wrapper is present.

Run any discovered task by its ID:

```bash
ldr run gradle.homeops-agent-api.bootRun
```

LDR executes the task directly without a shell and streams its output. It
resolves Node and Go commands to an absolute executable in the current user's
runtime store, rather than depending on the shell's PATH. Node uses
`package.json` `engines.node` when declared (otherwise Node 24 is the default);
Java uses the project's declared major where recognized and otherwise Java 21.

For long-running tasks, use the process manager:

```bash
ldr start gradle.homeops-agent-api.bootRun
ldr ps
ldr ps --json
ldr logs <process-id>
ldr stop <process-id>
ldr restart <process-id>
ldr cleanup
ldr cleanup --yes
ldr doctor
ldr doctor --json
ldr alias api gradle.homeops-agent-api.bootRun
```

LDR writes process metadata and logs only under `.ldr/state/`.

## Interactive launcher

Running `ldr` with no command opens an interactive task launcher in a terminal.
Tasks are organized as `folder → module → adapter → command`: for example,
`project root → homeops-agent-api → GRADLE → bootRun`. It opens like Finder:
the first pane lists project modules plus root-level tools; use Right or Enter
to open an item and Left to go back. The current path stays visible in the
compact breadcrumb at the top. Enter on a command runs it; `/` filters and `q`
quits. Gradle panes place `bootRun`, `build`, `clean`, and `test` at the top
as favorites, then organize the remaining commands using Gradle's task groups.
After a command exits (including a server stopped with Ctrl+C), LDR first opens
a scrollable final-log screen positioned at the end of the output. Press Enter
to run the same command again, or ← to return to the task list at the same
location. Foreground logs remain under `.ldr/state/logs/foreground/`. `ldr tui`
opens the same launcher explicitly.

## Profiles

Clone a discovered task into a user-managed profile:

```bash
ldr profile clone gradle.homeops-agent-api.bootRun agent-api-local
ldr profile show agent-api-local
ldr run agent-api-local
```

Profiles live in `.ldr/profiles/`, survive discovery refreshes, and extend the
base task rather than duplicating its command. Profile TOML supports
`env_from = [".env.local"]`, `[env]` overrides, and `[args]` `prepend` and
`append` arrays. Environment precedence is shell, env files in declaration
order, then explicit profile values. `profile show` masks values. A profile
whose base task disappears remains on disk and is shown as `BROKEN`.

## Project Configuration

Projects may define optional discovery settings in `.ldr/project.toml`:

```toml
version = 1
default_profile = "backend-local"
ignore = ["vendor", "generated"]

[runtimes]
java = "21"
node = "24"
```

The file is optional; changes invalidate the discovery cache. LDR stores aliases
and recent task IDs under `.ldr/state/`.

## Runtime store

Installed SDKs are user data, never project data. Their default locations are:

| Platform | Runtime root |
| --- | --- |
| macOS | `~/Library/Application Support/local-dev-runner/runtimes` |
| Linux | `$XDG_DATA_HOME/local-dev-runner/runtimes` or `~/.local/share/local-dev-runner/runtimes` |
| Windows | `%LOCALAPPDATA%\\local-dev-runner\\runtimes` |

Set `LDR_RUNTIME_HOME` to use another location.

Stable runtime paths are:

```text
java/<major>   # e.g. java/21
node/<major>   # e.g. node/24
go/1.<minor>   # e.g. go/1.26
```

Go projects are resolved from `go.mod` or `go.work`: `go 1.26.4` is a minimum
requirement, while `toolchain go1.26.8` is a preferred exact toolchain. A newer
installed patch meets an older patch-level minimum. The detailed policy is in
[docs/runtime-store.md](docs/runtime-store.md).

## Platform support

The current development and installation flow has been verified on macOS.
The code has platform-aware runtime-store defaults for macOS, Linux, and
Windows, but those operating systems have not yet been tested end-to-end.

## Development

```bash
go test ./...
gofmt -w ./cmd ./internal
```

Cross-platform compile checks can be run with `CGO_ENABLED=0 GOOS=windows
GOARCH=amd64 go test -c ./internal/cli`.

### Branch and PR workflow

Start from the latest `main` and create a dated integration branch:

```text
dev/{yyyy.mm.dd}
```

Create feature branches from it as `feature/issue-{issueNo}` and open feature
PRs into the dated `dev` branch. Run the full verification checklist and update
docs on `dev`. Before `dev` → `main`, an independent sub-agent must review the
combined diff, documentation, linked issues, and release impact.

After merging into `main`, tag the main commit automatically. Releases are
built from the tag, with both macOS architecture archives and checksums
attached to the GitHub Release. Update the release information in
`swsw1005/home-tab` as the final step.

### Release verification

Run `gofmt -w ./cmd ./internal`, `go test ./...`, and `git diff --check`.
Build both macOS architectures with `CGO_ENABLED=0`, `-trimpath`, and stripped
linker flags. Package `ldr_<version>_darwin_arm64.tar.gz` and
`ldr_<version>_darwin_amd64.tar.gz`, generate
`ldr_<version>_SHA256SUMS.txt`, attach all three files to the matching release,
and run `ldr --version` from the native archive.

## Roadmap

1. Release packaging and distribution improvements
2. More project-specific discovery adapters
3. Expanded runtime and project configuration support

See [local-dev-runner-initial-plan.md](local-dev-runner-initial-plan.md) for
the full initial plan.
