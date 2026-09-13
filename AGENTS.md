# Local Dev Runner instructions

## Runtime selection

- `~/Library/Application Support/local-dev-runner/runtimes` contains the Java,
  Node, and Go runtimes used for development.
- Select installed runtimes using their major-version paths.

## Releases

- A release is not complete until prebuilt macOS binaries are attached to its
  GitHub Release. End users must not need Go to install LDR.
- Before creating a release, run the complete Go test suite.
- Build from the release commit/tag for both supported macOS architectures with
  `CGO_ENABLED=0`, `-trimpath`, and stripped linker flags:
  - `darwin_arm64` for Apple Silicon
  - `darwin_amd64` for Intel Macs
- Package the executable as
  `ldr_<version>_darwin_arm64.tar.gz` and
  `ldr_<version>_darwin_amd64.tar.gz`.
- Generate and attach `ldr_<version>_SHA256SUMS.txt` containing the SHA-256
  checksums of both archives.
- Upload both archives and the checksum file to the matching GitHub Release,
  then extract and run at least the native-architecture archive to verify that
  `ldr --version` reports the release version.
