# Repository Guidelines

## Project Structure

LDR is a Go CLI. The entry point is `cmd/ldr`; application behavior is split
into focused packages under `internal/`:

- `cli`: command routing and user-facing output
- `discovery`: project adapters and cached task registry
- `execution` and `process`: task execution and lifecycle management
- `profile`, `project`, `state`, and `runtimes`: configuration and persistence
- `tui`: interactive terminal launcher

Tests live beside implementation files as `*_test.go`. Project-local runtime
state is generated under `.ldr/` and should not be committed.

## Build, Test, and Development

```bash
go build -o ./bin/ldr ./cmd/ldr
go test ./...
gofmt -w ./cmd ./internal
CGO_ENABLED=0 GOOS=windows GOARCH=amd64 go test -c ./internal/cli
```

Run the local CLI with `go run ./cmd/ldr --help`. Use the managed runtime
directories under `~/Library/Application Support/local-dev-runner/runtimes`
when a project requires a specific Java, Node, or Go major version.

## Coding and Naming

Use standard `gofmt` formatting, tabs for Go indentation, and small packages
with explicit interfaces at integration boundaries. Exported types and
functions require clear Go doc comments. Use `PascalCase` for exported names,
`camelCase` for local names, and descriptive test names such as
`TestReconcileMarksMissingProcessAsOrphaned`.

## Testing Guidelines

Add focused unit tests beside every behavior change. Keep platform-specific
process tests guarded by `runtime.GOOS`; use temporary directories and fixtures
instead of repository state. Before opening a PR, run `go test ./...`,
`git diff --check`, and relevant cross-compilation checks.

## Commits and Pull Requests

Use concise imperative commit subjects with a conventional prefix, for example
`feat: add task search` or `fix: reconcile stale process state`. PRs should
describe behavior, compatibility impact, and validation commands; link the
corresponding GitHub issue. Include CLI examples or screenshots when changing
user-visible output.

## Releases

Run the full Go test suite before releasing. Build from the release tag for
`darwin_arm64` and `darwin_amd64` with `CGO_ENABLED=0`, `-trimpath`, and
stripped linker flags. Attach both archives and a SHA-256 checksum file to the
GitHub Release, then verify the native archive with `ldr --version`.
