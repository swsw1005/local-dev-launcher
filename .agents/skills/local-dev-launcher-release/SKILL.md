---
name: local-dev-launcher-release
description: Follow the repository's dated dev-branch, feature PR, verification, tagging, release, and home-tab update workflow for LDR changes.
---

# Local Dev Launcher Release Workflow

Use this skill when developing an issue through integration and release, or
when preparing a versioned LDR release.

## Branching

1. Fetch the latest `main`.
2. Create the cycle branch `dev/{yyyy.mm.dd}` from `main`.
3. Create each feature branch from that dev branch using
   `feature/issue-{issueNo}`.
4. Open feature PRs into the dated dev branch. Keep issue scope, tests, and
   docs in the same change.

Before merging dev into main, request an independent sub-agent review covering
the combined diff, documentation, linked issues, compatibility, and release
impact. Do not approve integration when required work remains.

## Verification

On the dev branch, run `gofmt -w ./cmd ./internal`, `go test ./...`,
`git diff --check`, and a clean `CGO_ENABLED=0 go build` with stripped linker
flags. Run relevant Windows compile checks and platform-specific tests. Update
README, AGENTS, CHANGELOG, and other user-facing docs before integration.

## Release

After main is merged, derive the version from the repository's CLI version,
create and push the matching `v<version>` tag automatically, and use that tag
as the release source. Build with `CGO_ENABLED=0`, `-trimpath`, and stripped
linker flags for `darwin_arm64` and `darwin_amd64`. Package exactly:

- `ldr_<version>_darwin_arm64.tar.gz`
- `ldr_<version>_darwin_amd64.tar.gz`
- `ldr_<version>_SHA256SUMS.txt`

Verify the native archive with `ldr --version`, verify checksums, and attach
all three assets to the matching GitHub Release. Finally update release
metadata in `swsw1005/home-tab` and verify that repository's change.

Never release from an untagged or dirty commit. If tag, release, asset upload,
or home-tab synchronization cannot be completed, stop and report the exact
blocking step.
