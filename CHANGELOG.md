# Changelog

## 0.8.0 — 2026-09-26

- Added a global LDR runtime guide (`~/.ldr/ldr_runtime_guide.md`) covering
  runtime store locations, release search, and installation.
- Added short runtime-guide links to global Codex and Claude Code instruction
  files, written only during normal startup in a detected agent environment.
- Added `ldr doctor --fix-agent-guidance` to create the guide and append
  missing links, and `ldr doctor --force-agent-guidance` (alias `--force`) to
  restore damaged managed blocks when marker boundaries are intact.

## 0.7.0 — 2026-09-20

- Added graceful process-group shutdown, restart, status reconciliation, and
  explicit orphan cleanup.
- Added `ldr ps --json` and `ldr doctor [--json]` diagnostics.
- Added profile env files, interpolation, argument overrides, and secret
  masking in profile output.
- Added `.ldr/project.toml` discovery configuration and cache invalidation.
- Added fuzzy task search, JSON search results, aliases, and recent tasks.
- Added Cargo, Makefile, and Docker Compose discovery adapters.
- Hardened state persistence with atomic JSON writes.
