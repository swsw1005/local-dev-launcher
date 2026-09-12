# Local Dev Runner (LDR) — Initial Project Plan

## 1. Project Overview

**Project name:** `local-dev-runner`  
**Binary / CLI name:** `ldr`

Local Dev Runner (LDR) is a zero-config local development runner for terminal-first development workflows.

The project is intended to replace the parts of an IDE that are still useful even when most development is performed through CLI coding agents such as Codex or Claude Code.

LDR should automatically understand a project, discover its runtimes, modules, and runnable tasks, cache that information locally, and provide a unified interface for humans and coding agents to run and manage local development processes.

The long-term goal is:

> Discover → Resolve → Run → Manage

LDR should become the local execution layer shared by both developers and coding agents.

---

## 2. Core Problem

In terminal-first and agent-driven development, a full IDE is often unnecessary.

However, IDEs still provide several convenient functions:

- Gradle task explorer
- Maven lifecycle/task explorer
- npm script explorer
- Run configurations
- Environment-variable-based run profiles
- Compound run configurations
- Start / stop / restart
- Console and logs
- Runtime/JDK/Node configuration visibility
- Module discovery

Without an IDE, users must remember commands such as:

```bash
./gradlew :manager-admin-api:bootRun
./gradlew :manager-worker:test
./mvnw -pl api -am package
npm run dev
pnpm test
```

The user should not have to manually maintain these commands.

LDR must discover them automatically.

---

## 3. Product Principles

### 3.1 Zero configuration by default

Running:

```bash
ldr
```

inside a project should be useful even when no LDR configuration exists.

Configuration files are optional overrides, not prerequisites.

---

### 3.2 Project-local metadata

LDR stores project-specific metadata under:

```text
.ldr/
```

This directory belongs to the local working copy.

Generated discovery results must not be mixed with user-created run profiles.

---

### 3.3 Generated state and user state must be separated

LDR-generated data may be deleted and regenerated at any time.

User-created profiles must never be deleted or overwritten by automatic discovery.

---

### 3.4 Humans and agents use the same execution model

The TUI, CLI, and future MCP server must all use the same internal task registry and execution service.

Do not implement separate execution logic for the TUI.

---

### 3.5 Prefer project wrappers

When available:

- `./gradlew` over system `gradle`
- `./mvnw` over system `mvn`

Node package managers should be resolved from project metadata and lock files.

---

### 3.6 Runtime installation is not an MVP responsibility

LDR resolves required runtimes and verifies whether they are usable.

It does **not** initially replace:

- mise
- asdf
- SDKMAN
- Homebrew
- nvm
- fnm

Example:

```text
Required: Java 21
Detected: Java 17

Status: unresolved
```

Future versions may integrate with an installed runtime manager, but LDR should not become a runtime installer in the first implementation.

---

## 4. High-Level Architecture

```text
                 Project Directory
                        │
                        ▼
               Project Discovery
                        │
         ┌──────────────┼──────────────┐
         │              │              │
      Gradle          Maven           Node
         │              │              │
         └──────────────┼──────────────┘
                        ▼
                Runtime Resolver
                        │
                        ▼
                  Task Registry
                        │
               Profile Resolver
                        │
                        ▼
                 Execution Service
                        │
                        ▼
                 Process Manager
                        │
         ┌──────────────┼──────────────┐
         │              │              │
        CLI            TUI       Future MCP
```

Suggested internal components:

```text
local-dev-runner/
├─ cmd/
│  └─ ldr/
├─ internal/
│  ├─ discovery/
│  ├─ adapters/
│  │  ├─ gradle/
│  │  ├─ maven/
│  │  └─ node/
│  ├─ runtime/
│  ├─ registry/
│  ├─ cache/
│  ├─ profile/
│  ├─ execution/
│  ├─ process/
│  ├─ tui/
│  └─ config/
└─ testdata/
```

Avoid unnecessary abstraction until required.

---

# 5. `.ldr/` Directory

When LDR is first executed in a supported project, it creates:

```text
.ldr/
├─ cache/
│  ├─ manifest.json
│  ├─ runtimes.json
│  ├─ modules.json
│  └─ tasks.json
│
├─ profiles/
│  ├─ backend-local.toml
│  └─ frontend-local.toml
│
├─ state/
│  └─ processes.json
│
└─ project.toml
```

Not every file must exist immediately.

The directory responsibilities are:

### `.ldr/cache/`

Owned entirely by LDR.

Contains generated discovery results.

It must be safe to delete at any time.

LDR must be able to reconstruct it.

### `.ldr/profiles/`

Owned by the user.

Contains manually created or cloned execution profiles.

Automatic discovery must never delete or overwrite these files.

### `.ldr/state/`

Ephemeral runtime state.

Examples:

- running process metadata
- process IDs
- log/session references

This state must not be treated as durable configuration.

### `.ldr/project.toml`

Optional project-level LDR configuration.

Zero-config operation must remain possible when this file does not exist.

---

# 6. Git Ignore Policy

The entire `.ldr/` directory is considered local project state in the initial implementation.

It should be ignored by Git.

The expected `.gitignore` entry is:

```gitignore
.ldr/
```

## 6.1 Automatic check

Every time a command is about to execute, LDR must verify whether `.ldr/` is effectively ignored by Git.

The check should preferably use Git itself rather than only string-matching `.gitignore`.

For example:

```bash
git check-ignore .ldr/
```

or an equivalent reliable mechanism.

This allows global excludes and nested `.gitignore` behavior to be respected.

---

## 6.2 Warning behavior

If the project is inside a Git repository and `.ldr/` is **not ignored**, command execution must not be silently performed.

Display a visible warning such as:

```text
WARNING: .ldr/ is not ignored by Git.

LDR stores generated cache, runtime state, and local execution profiles
inside .ldr/. Committing this directory is not recommended.

Add the following entry to .gitignore:

    .ldr/

Press Enter to continue, or Ctrl+C to cancel.
```

For interactive TUI execution, require explicit confirmation.

For non-interactive execution, do not hang waiting for input.

Suggested behavior:

```bash
ldr run ...
```

- interactive terminal: warn and ask for confirmation
- non-interactive/CI/agent mode: emit warning to stderr and continue only when an explicit flag is supplied

Suggested override:

```bash
ldr run <task> --allow-unignored-ldr
```

The exact flag name may be adjusted during implementation, but an explicit bypass must exist.

---

## 6.3 Initialization UX

When `.ldr/` is created for the first time, LDR may suggest:

```text
Add ".ldr/" to .gitignore? [Y/n]
```

Do not modify `.gitignore` silently.

---

# 7. Discovery Sources

LDR searches the current directory and determines the project root.

Supported project types for MVP:

1. Gradle
2. Maven
3. Node/npm-compatible projects

Future support:

- pnpm workspaces
- npm workspaces
- yarn workspaces
- Bun
- Docker Compose
- just
- mise
- Taskfile
- Go
- Python
- Cargo

---

# 8. Gradle Discovery

Detect:

```text
gradlew
gradlew.bat
settings.gradle
settings.gradle.kts
build.gradle
build.gradle.kts
gradle.properties
```

Wrapper priority:

```text
./gradlew
gradlew.bat
gradle
```

Discover:

- root project
- subprojects
- task names
- task paths
- task groups
- descriptions where available

Possible discovery commands:

```bash
./gradlew projects
./gradlew tasks --all --console=plain
```

Stable task examples:

```text
gradle.root.clean
gradle.root.build
gradle.manager-admin-api.bootRun
gradle.manager-admin-api.test
```

Stored execution form:

```json
{
  "id": "gradle.manager-admin-api.bootRun",
  "adapter": "gradle",
  "module": "manager-admin-api",
  "name": "bootRun",
  "cwd": ".",
  "command": "./gradlew",
  "args": [
    ":manager-admin-api:bootRun"
  ]
}
```

Do not store only a shell command string.

Store command and arguments separately.

---

# 9. Maven Discovery

Detect:

```text
mvnw
mvnw.cmd
pom.xml
```

Wrapper priority:

```text
./mvnw
mvnw.cmd
mvn
```

Parse `pom.xml` to discover:

- artifactId
- packaging
- modules
- parent relationships

Initially expose common lifecycle tasks:

```text
clean
compile
test
package
verify
install
```

Example:

```text
maven.api.test
maven.api.package
```

Possible execution:

```bash
./mvnw -pl api test
```

Support for `-am` should be possible as an option/profile override.

---

# 10. Node Discovery

Detect:

```text
package.json
package-lock.json
pnpm-lock.yaml
yarn.lock
bun.lock
```

Package manager resolution priority should derive from explicit project metadata where possible.

Possible signals:

- `packageManager` field in `package.json`
- lock files
- installed executable availability

Example:

```json
{
  "scripts": {
    "dev": "next dev",
    "build": "next build",
    "test": "vitest",
    "lint": "eslint ."
  }
}
```

Generated tasks:

```text
node.frontend.dev
node.frontend.build
node.frontend.test
node.frontend.lint
```

---

# 11. Runtime Resolver

The runtime resolver is an internal component of LDR.

Its responsibility is:

> Determine what runtime and toolchain the project expects and whether that runtime can currently be used.

Examples of sources:

### Java

- Gradle toolchains
- Maven compiler properties/plugins
- `.java-version`
- `.sdkmanrc`
- `.tool-versions`
- `mise.toml`

### Node

- `package.json` engines
- `packageManager`
- `.nvmrc`
- `.node-version`
- `.tool-versions`
- `mise.toml`

### Go

- `go.mod` `go` directive (minimum required Go version)
- `go.mod` / `go.work` `toolchain` directive (preferred exact toolchain)
- `go.work` `go` directive

Go uses `1.<minor>.<patch>` release versions. LDR stores one stable path per
minor family (`go/1.26`) that points to its selected patch release. The
resolver must still compare a patch-level `go` requirement when one is declared
and must report a preferred exact `toolchain go1.26.8` when it is unavailable.
As with Java and Node, MVP behavior is resolution and verification only; it
does not download or update Go toolchains.

Possible output:

```json
{
  "type": "java",
  "required": "21",
  "resolved": "21.0.8",
  "executable": "/path/to/java",
  "status": "ready"
}
```

If unresolved:

```text
Java 21 required
Java 17 detected

Status: unresolved
```

Do not automatically install runtimes in MVP.

---

# 12. Manifest and Checksums

LDR caches all files that affect discovery.

Example:

```text
settings.gradle.kts
build.gradle.kts
manager-admin-api/build.gradle.kts
manager-worker/build.gradle.kts
frontend/package.json
frontend/package-lock.json
```

For every relevant source file, cache:

- relative path
- adapter
- checksum
- optionally size
- optionally modification timestamp for fast pre-check

Example manifest:

```json
{
  "version": 1,
  "sources": [
    {
      "path": "settings.gradle.kts",
      "adapter": "gradle",
      "checksum": "sha256:..."
    },
    {
      "path": "manager-admin-api/build.gradle.kts",
      "adapter": "gradle",
      "checksum": "sha256:..."
    },
    {
      "path": "frontend/package.json",
      "adapter": "node",
      "checksum": "sha256:..."
    }
  ]
}
```

SHA-256 is sufficient.

---

# 13. Cache Refresh Rules

When `ldr` starts:

```text
Discover source files
        ↓
Compare cached checksums
        ↓
No changes
        ↓
Use existing task/runtime/module cache
```

When a checksum changes:

```text
Relevant source changed
        ↓
Invalidate affected generated discovery cache
        ↓
Run discovery again
        ↓
Rebuild generated task registry
        ↓
Keep all user profiles
```

For MVP it is acceptable to rebuild all generated discovery state whenever any tracked checksum changes.

The architecture should allow adapter-specific invalidation later.

Example future optimization:

```text
frontend/package.json changed
        ↓
invalidate Node adapter only

Gradle cache remains valid
```

---

# 14. Task Registry

All discovered tasks are normalized into one registry.

Example:

```text
gradle.root.clean
gradle.root.build
gradle.admin-api.bootRun
gradle.admin-api.test
maven.api.package
node.frontend.dev
node.frontend.build
```

A task should contain at least:

```go
type Task struct {
    ID          string
    Name        string
    Group       string
    Description string

    Adapter     string
    Module      string

    WorkingDir  string
    Command     string
    Args        []string
}
```

Prefer direct subprocess execution:

```go
exec.CommandContext(ctx, command, args...)
```

Avoid unnecessary shell execution.

---

# 15. Execution Profiles

A discovered task may be cloned into a user profile.

Example:

```bash
ldr profile clone gradle.manager-admin-api.bootRun backend-local
```

Creates:

```text
.ldr/profiles/backend-local.toml
```

Example:

```toml
version = 1

name = "Backend Local"
extends = "gradle.manager-admin-api.bootRun"

[env]
SPRING_PROFILES_ACTIVE = "local"
SERVER_PORT = "8081"

[args]
append = [
  "--args=--spring.config.additional-location=./config/local/"
]
```

The profile should reference a generated task using `extends`.

Do not copy and permanently detach the generated command unless the user explicitly chooses to create a fully custom profile.

---

# 16. Profile Persistence

Profiles are user-managed data.

When discovery cache is rebuilt:

```text
cache/
    regenerated

profiles/
    untouched
```

Profiles must survive:

- checksum changes
- task cache rebuilds
- runtime discovery changes
- LDR upgrades where compatible

---

# 17. Missing Base Task

If a profile references:

```toml
extends = "gradle.manager-api.bootRun"
```

but the task no longer exists after rediscovery, the profile must remain on disk.

Mark it as broken:

```text
Backend Local
BROKEN

Base task not found:
gradle.manager-api.bootRun
```

Do not silently remap it.

A future feature may suggest similar tasks:

```text
Possible replacement:
gradle.manager-admin-api.bootRun
```

but user confirmation is required.

---

# 18. Environment Variables

Profiles may add or override environment variables.

Example:

```toml
[env]
SPRING_PROFILES_ACTIVE = "local"
SERVER_PORT = "8081"
```

Support external environment files:

```toml
[env_from]
files = [
  ".env.local"
]
```

Support environment interpolation where practical:

```toml
[env]
DB_PASSWORD = "${DB_PASSWORD}"
```

Avoid encouraging secrets to be written directly inside `.ldr/profiles`.

---

# 19. Project Configuration

Optional:

```text
.ldr/project.toml
```

Example:

```toml
version = 1

[project]
name = "homeops"

[discovery]
gradle = true
maven = true
node = true

[discovery.ignore]
paths = [
  ".git",
  ".ldr",
  "node_modules",
  "build",
  "dist",
  "target"
]
```

The absence of this file must not prevent normal operation.

---

# 20. CLI Design

Initial CLI:

```bash
ldr
```

Launch TUI.

```bash
ldr list
```

List discovered tasks.

```bash
ldr list --json
```

Machine-readable output.

```bash
ldr run <task-id>
```

Run discovered task.

```bash
ldr run <profile-name>
```

Run user profile.

```bash
ldr refresh
```

Force discovery and cache regeneration.

```bash
ldr profile list
ldr profile clone <task-id> <profile-name>
ldr profile show <profile-name>
```

Later:

```bash
ldr ps
ldr logs <process>
ldr stop <process>
ldr restart <process>
```

---

# 21. TUI

The TUI is a client of the same task registry and execution service used by the CLI.

Example:

```text
┌─ local-dev-runner ──────────────────────────────────────┐
│ Java 21 | Gradle 9.x | Node 24 | npm                   │
├────────────────┬────────────────────────────────────────┤
│ Modules        │ Tasks                                  │
│                │                                        │
│ root           │ Gradle                                 │
│ admin-api      │   clean                                │
│ worker         │   build                                │
│ frontend       │   test                                 │
│                │                                        │
│                │ admin-api                              │
│                │   bootRun                              │
│                │   test                                 │
│                │                                        │
│                │ Profiles                               │
│                │   Backend Local                        │
├────────────────┴────────────────────────────────────────┤
│ / search | Enter run | R refresh | q quit              │
└─────────────────────────────────────────────────────────┘
```

MVP controls:

```text
↑ / ↓      navigate
Enter      run
/          search
R          refresh
q          quit
```

Fuzzy search is desirable.

---

# 22. Process Manager

Long-running process management is a core target, but it does not need to be implemented before basic discovery and execution are reliable.

Examples:

```text
backend      RUNNING    pid 38120
frontend     RUNNING    pid 38155
test         SUCCESS    exit 0
```

Future commands:

```bash
ldr ps
ldr stop backend
ldr restart backend
ldr logs backend
```

Future TUI controls:

```text
Enter     run
S         stop
R         restart
L         logs
```

Process execution must handle signals and child process termination correctly.

---

# 23. Agent Integration

CLI output should be designed so coding agents can use LDR without TUI interaction.

Required future-friendly commands:

```bash
ldr list --json
ldr run <task-id> --json
ldr status --json
ldr logs <process> --json
```

Long-term MCP interface:

```text
list_tasks()
run_task(task_id)
list_processes()
get_process_status(process_id)
get_logs(process_id)
stop_process(process_id)
```

MCP is not part of the first MVP.

---

# 24. Language and Libraries

Use Go.

Reasons:

- single binary
- good subprocess handling
- fast startup
- macOS/Linux/Windows distribution
- good TUI ecosystem
- straightforward JSON/XML/TOML parsing

For TUI, evaluate:

- Bubble Tea
- Bubbles
- Lip Gloss

Do not add TUI dependencies until the CLI core works.

---

# 25. Command Execution Safety

Use structured command execution.

Prefer:

```go
exec.CommandContext(ctx, command, args...)
```

Do not concatenate arbitrary task input into a shell command unless shell execution is explicitly required.

Environment variables should be constructed from:

```text
current environment
    +
profile env
    +
explicit CLI overrides
```

The exact precedence must be documented.

---

# 26. Cross-Platform Concerns

Support should be structurally possible for:

- macOS
- Linux
- Windows

Consider:

```text
gradlew       vs gradlew.bat
mvnw          vs mvnw.cmd
path separators
signal behavior
process groups
shell differences
```

Initial development may target macOS/Linux first, but architecture must not unnecessarily block Windows.

---

# 27. Testing Strategy

Create fixture projects:

```text
testdata/
├─ gradle-single/
├─ gradle-multi/
├─ maven-single/
├─ maven-multi/
├─ node-npm/
├─ spring-gradle-multi/
└─ mixed-backend-frontend/
```

Separate:

- parser/unit tests
- discovery tests
- integration tests requiring actual Gradle/Maven/Node

External command execution should be abstracted.

Example:

```go
type CommandRunner interface {
    Run(
        ctx context.Context,
        cwd string,
        command string,
        args ...string,
    ) (CommandResult, error)
}
```

---

# 28. MVP Milestones

## Milestone 1 — Repository and Core Model

Implement:

- Go module
- `ldr` CLI skeleton
- project root detection
- `.ldr/` directory initialization
- Git ignore detection
- Git ignore warning mechanism
- cache/profile/state directory separation
- shared domain models
- unit tests

No TUI yet.

Acceptance criteria:

```bash
ldr init
```

or first `ldr` execution creates the required local structure.

If `.ldr/` is not ignored by Git, the user sees a clear warning.

---

## Milestone 2 — Discovery

Implement:

- Gradle detector
- Maven detector
- Node detector
- module discovery
- runtime source-file tracking
- SHA-256 checksum manifest
- task registry generation
- cache persistence

Command:

```bash
ldr list
```

must work.

---

## Milestone 3 — Cache Invalidation

Implement:

- cached checksum comparison
- automatic rediscovery when tracked files change
- unchanged projects reuse cache
- user profiles survive all refreshes

Acceptance scenario:

1. run `ldr list`
2. cache created
3. edit `build.gradle.kts`
4. run `ldr list`
5. Gradle tasks are rediscovered
6. `.ldr/profiles/*` remains unchanged

---

## Milestone 4 — Task Execution

Implement:

```bash
ldr run <task-id>
```

Requirements:

- correct working directory
- wrapper preference
- stdout/stderr streaming
- exit code propagation
- Git-ignore safety check before execution

---

## Milestone 5 — Profiles

Implement:

```bash
ldr profile clone <task-id> <name>
ldr profile list
ldr profile show <name>
ldr run <profile-name>
```

Support:

- `extends`
- environment variables
- appended/prepended args
- `.env` references
- broken base-task detection

---

## Milestone 6 — TUI

Implement the interactive task launcher.

The TUI must depend on the existing registry and execution services.

Do not duplicate Gradle/Maven/Node discovery code inside the TUI.

---

## Milestone 7 — Process Manager

Implement long-running process management:

- start
- stop
- restart
- status
- logs

Persist only the minimum process metadata required in `.ldr/state/`.

---

# 29. Out of Scope for Initial MVP

Do not implement yet:

- automatic JDK installation
- automatic Node installation
- full replacement for mise/asdf/sdkman
- remote runners
- container orchestration platform
- cloud execution
- CI runner
- MCP server
- IDE plugins
- complex plugin marketplace
- distributed process management

---

# 30. Definition of Success

The first useful version should support the following workflow:

```bash
cd my-project
ldr
```

LDR should:

1. detect the project
2. create `.ldr/`
3. verify `.ldr/` is Git-ignored
4. discover Gradle/Maven/Node structure
5. determine relevant runtimes
6. cache source checksums
7. generate runnable task definitions
8. present tasks to the user
9. execute selected tasks
10. automatically refresh discovery when relevant build files change
11. preserve user-created profiles during every refresh

A developer should no longer need an IDE solely to browse and execute Gradle/Maven/npm tasks.

---

# 31. Codex Development Rules

When implementing this project:

1. Work milestone by milestone.
2. Do not implement later phases early unless required by the current architecture.
3. Prefer simple code over premature plugin systems.
4. Keep generated state separate from user-managed state.
5. Never overwrite user profiles during automatic refresh.
6. Treat `.ldr/cache/` as disposable.
7. Treat `.ldr/profiles/` as user data.
8. Verify `.ldr/` Git-ignore status before task execution.
9. Prefer project wrappers over globally installed tools.
10. Do not rely on shell command string concatenation when direct subprocess execution is possible.
11. Add tests with each feature.
12. Run formatting and tests after each milestone.
13. Keep CLI/TUI/business logic separated so a future MCP layer can reuse the same services.
14. Do not silently alter `.gitignore`; ask or warn.
15. Do not silently repair broken profile references.
16. Preserve backwards compatibility of profile files when reasonably possible.
17. Document any intentional deviation from this plan before implementing it.

---

# 32. First Codex Task

Start with **Milestone 1 only**.

Before coding:

1. inspect the repository
2. if empty, initialize the Go project
3. propose a concise package structure
4. identify the minimum domain models required
5. implement the repository/core skeleton
6. implement `.ldr/` initialization
7. implement Git-ignore detection and warning behavior
8. add unit tests
9. run tests and formatting
10. summarize what was implemented and what remains for Milestone 2

Do not implement Gradle/Maven/npm discovery until Milestone 1 is complete and tested.
