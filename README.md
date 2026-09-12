# Local Dev Runner (LDR)

Local Dev Runner is a terminal-first local development runner. It is designed
to discover project tasks and provide one execution model for developers,
coding agents, and a future TUI.

```text
Discover → Resolve → Run → Manage
```

## Status

The project is in its first implementation milestone. It currently provides:

- `ldr init` to create isolated project-local state
- Git ignore detection and a clear warning for unignored `.ldr/` directories
- shared domain models for future discovery, profiles, and execution
- user-level runtime-store paths
- Go runtime requirement parsing and resolution (`go.mod` / `go.work`)

Gradle, Maven, Node task discovery, task execution, profiles, TUI, and process
management are planned but are not available yet.

## Install from source

LDR is written in Go. Build the binary into a directory already on your PATH:

```bash
go build -o ~/bin/ldr ./cmd/ldr
```

Then run it from any project directory:

```bash
ldr
ldr init
```

If `~/bin` is not on your PATH, add this to your shell configuration:

```bash
export PATH="$HOME/bin:$PATH"
```

## Project-local state

`ldr init` creates only local state in the detected project root:

```text
.ldr/
├── cache/      # LDR-generated and disposable
├── profiles/   # user-managed; never overwritten by discovery
└── state/      # ephemeral process state
```

LDR never silently changes `.gitignore`. In a Git worktree it warns when
`.ldr/` is not ignored; add the following entry yourself:

```gitignore
.ldr/
```

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
