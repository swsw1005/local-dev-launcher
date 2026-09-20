# Changelog

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
