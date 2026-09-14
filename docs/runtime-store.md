# Runtime-store policy

LDR keeps shared runtimes in per-user application data rather than a project
checkout. The default path is platform specific:

| Platform | Default runtime root |
| --- | --- |
| macOS | `~/Library/Application Support/local-dev-runner/runtimes` |
| Linux | `$XDG_DATA_HOME/local-dev-runner/runtimes`, falling back to `~/.local/share/local-dev-runner/runtimes` |
| Windows | `%LOCALAPPDATA%\\local-dev-runner\\runtimes` |

Set `LDR_RUNTIME_HOME` to override this location, for example for a shared
volume. Project-local `.ldr/` directories never contain installed runtimes.

The store exposes stable paths by runtime family:

```text
runtimes/
├── java/<major>
├── node/<major>
└── go/1.<minor>
```

## Go resolution policy

LDR discovers `go.mod` and `go.work`.

- A `go 1.26` or `go 1.26.4` line is a **minimum required version**. Its
  language family is `1.26`, but a patch-level minimum must be met when it is
  present.
- A `toolchain go1.26.8` line is a **preferred exact toolchain**. It takes
  precedence for selection when available, while the `go` line remains the
  hard minimum.
- The managed stable location is therefore `go/1.26`, pointing to the latest
  installed patch in that family. LDR records both the requested version and
  the resolved exact version in its runtime cache.

## Installation and updates

On macOS, `ldr install` downloads runtimes into these stable paths and replaces
only the selected family after verifying the vendor-provided SHA-256 checksum.
It never installs into a project directory. `ldr init-shell` is the separate,
opt-in command that configures shared Bash/Zsh PATH handling.

- `ldr install java 21`, `ldr install node 24`, and `ldr install go 1.26`
  install or update one family.
- `ldr install java --lts` and `ldr install node --lts` install the five newest
  LTS families from the vendor release metadata. Families without an archive
  for the current macOS architecture are skipped rather than failing the whole
  LTS installation.
- `ldr install all` installs the latest Java, Node, and Go families;
  `ldr install all --lts` uses LTS families for Java and Node.
- Node installation also installs `pnpm` in that Node family. Activating the
  family links `node`, `npm`, `npx`, and `pnpm` under `~/bin`.
