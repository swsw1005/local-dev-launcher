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

## Development Workflow

Start each development cycle from the latest `main` by creating a dated
integration branch:

```text
dev/{yyyy.mm.dd}
```

Create feature branches from that branch using `feature/issue-{issueNo}`.
Develop and validate the feature there, then open a PR into the dated `dev`
branch. Keep issue scope, implementation, tests, and documentation aligned.

Before merging `dev` into `main`, run the complete test and verification
checklist on `dev`, update the final documentation, and obtain an independent
sub-agent review of the diff, docs, linked issues, and release impact. Merge
only after that review approves the integration.

After `main` is merged, create the release tag automatically from the main
commit. Build release artifacts from that tag, attach them to the GitHub
Release, and update release information in `swsw1005/home-tab`.

## Verification Checklist

Run `gofmt`, `go test ./...`, `git diff --check`, and a clean `CGO_ENABLED=0`
build. For releases, build both `darwin_arm64` and `darwin_amd64`, verify the
native archive with `ldr --version`, and validate every archive with the
generated SHA-256 file.

## Releases

Use the release tag as the sole source of truth. Run the full Go test suite
before releasing. Build from the release tag for
`darwin_arm64` and `darwin_amd64` with `CGO_ENABLED=0`, `-trimpath`, and
stripped linker flags. Attach both archives and a SHA-256 checksum file to the
GitHub Release, then verify the native archive with `ldr --version`. Finish by
updating the corresponding release metadata in `swsw1005/home-tab`.
