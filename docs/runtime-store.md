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

The store exposes stable paths while retaining the runtime manager's actual
installations below `mise/installs/`:

```text
runtimes/
├── java/<major>       -> mise/installs/java/<exact-version>
├── node/<major>       -> mise/installs/node/<exact-version>
├── go/1.<minor>       -> mise/installs/go/<exact-version>
└── mise/installs/
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

The first MVP resolves and verifies these runtimes only. It does not download,
upgrade, or silently switch them.
