# Local Dev Runner (LDR)

Local Dev Runner is a terminal-first local development runner. It is designed
to discover project tasks and provide one execution model for developers,
coding agents, and a future TUI.

```text
Discover → Resolve → Run → Manage
```

## Status

The MVP CLI core currently provides:

- `ldr init` to create isolated project-local state
- Git ignore detection and a clear warning for unignored `.ldr/` directories
- normalized task registry and SHA-256 discovery cache
- direct structured task execution with streamed output and exit-code propagation
- user profiles with environment and argument overrides
- user-level runtime-store paths
- Go runtime requirement parsing and resolution (`go.mod` / `go.work`)

Static Gradle, Maven, Node, and Go task discovery is available. TUI and
long-running process management are the next milestones.

## Install from source

LDR is written in Go. Build the binary into a directory already on your PATH:

```bash
go build -o ~/bin/ldr ./cmd/ldr
```

Then run it from any project directory:

```bash
ldr
ldr init
ldr list
ldr list --json
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

If `~/bin` is not on your PATH, add this to your shell configuration:

```bash
export PATH="$HOME/bin:$PATH"
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
ldr logs <process-id>
ldr stop <process-id>
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
After a command exits (including a server stopped with Ctrl+C), LDR returns to
that same selected command; press Enter to run it again. `ldr tui` opens the
same launcher explicitly.

## Profiles

Clone a discovered task into a user-managed profile:

```bash
ldr profile clone gradle.homeops-agent-api.bootRun agent-api-local
ldr profile show agent-api-local
ldr run agent-api-local
```

Profiles live in `.ldr/profiles/`, survive discovery refreshes, and extend the
base task rather than duplicating its command. The initial TOML format supports
`[env]` overrides plus `[args]` `prepend` and `append` arrays. A profile whose
base task disappears remains on disk and is shown as `BROKEN`.

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
```

The project deliberately uses the Go standard library only at this stage.

## Roadmap

1. Core state, Git safety, and shared runtime model
2. Gradle, Maven, Node, and Go discovery with cache invalidation
3. Structured task execution
4. User execution profiles
5. TUI and process management

See [local-dev-runner-initial-plan.md](local-dev-runner-initial-plan.md) for
the full initial plan.
