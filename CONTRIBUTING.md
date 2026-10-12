# Contributing to Sentra

Sentra is a lightweight, daemonless, OCI-compliant container runtime in pure
Go. This file covers how to build, test, and submit changes.

## Prerequisites

- **Go 1.25** or newer.
- **Linux** for running containers (namespaces, cgroups v2, overlayfs).
- **`runc`** on `PATH` for container execution.
- **`ip`** and **`iptables`** for networking (rootful runs).

## Project layout

```
sentra/
├── cmd/sentra/            # thin CLI entrypoint
├── internal/
│   ├── cli/               # command implementations
│   ├── config/            # Sentrafile parser
│   ├── build/             # build engine (DAG, cache, executor, push)
│   ├── runtime/           # runc wrapper, namespaces, cgroups, state
│   ├── image/             # OCI pull/push, registry
│   ├── overlayfs/         # mount and layer store
│   ├── security/          # rootless, seccomp, hardening
│   └── network/           # bridge, port mapping, NAT
├── scripts/build.sh       # reproducible release build
├── docs/                  # Sentrafile syntax, architecture, comparison
└── website/               # landing page
```

`internal/` stays internal. Only put a type in `pkg/` if it genuinely needs to
be importable outside the module.

## Build

```sh
# dev build
go build -o sentra ./cmd/sentra

# release build (stripped, reproducible, size-budget gated)
bash scripts/build.sh
# → dist/sentra-linux-amd64

# release build with UPX compression
bash scripts/build.sh --pack
# → dist/sentra-linux-amd64-packed
```

The release build enforces the project's binary-size budget: **≤ 15 MiB
unpacked, ≤ 6 MiB UPX-packed**. If a change pushes the binary over the limit,
the build fails — audit new imports.

`CGO_ENABLED=0` is always set. No cgo, no embedded C, no Rust FFI.

## Test

```sh
# run all tests
go test ./...

# run tests for one package
go test ./internal/config/...

# verbose
go test -v ./internal/build/...

# run with the race detector (Linux)
go test -race ./internal/build/...
```

Tests live next to the code they cover (`foo_test.go` alongside `foo.go`).
Networking and runtime tests use dry-run modes where possible so they don't
require root.

## Lint

There is no separate linter configured. `go vet` is the baseline:

```sh
go vet ./...
```

Before submitting, run:

```sh
go build ./...
go vet ./...
go test ./...
```

## Coding conventions

- **Pure Go, `CGO_ENABLED=0`.** No cgo, no embedded C.
- **No daemon.** Never introduce a background service, socket server, or
  long-running process as an architectural requirement.
- **OCI images only.** No custom image format.
- **Wrap `runc`.** Do not hand-roll namespace/cgroup syscalls from scratch.
- **cgroups v2 only.** No v1 fallback.
- **overlayfs only.** No other storage drivers.
- **One network implementation.** No driver zoo.
- **Binary size is a first-class constraint.** Flag anything that pulls in
  heavy dependencies, reflection-heavy libraries, or non-stdlib HTTP clients.
- **CLI:** stdlib `flag` (or `cobra` if justified). HTTP: stdlib `net/http`
  only.
- **No AI-powered features.** No `sentra fix`, `sentra optimize`, etc.

## Submitting a change

1. Fork and create a feature branch.
2. Make your change with tests.
3. Run `go build ./... && go vet ./... && go test ./...`.
4. If you changed dependencies, verify the binary stays under the size budget
   with `bash scripts/build.sh`.
5. Open a pull request with a clear description of what changed and why.

## Reporting issues

Use the issue templates:

- **Bug report** — what happened, what you expected, steps to reproduce, and
  the Sentra version/commit.
- **Feature request** — what you need and why.

See [.github/ISSUE_TEMPLATE/](.github/ISSUE_TEMPLATE/).

## License

Sentra is open source. See the `LICENSE` file for details.
