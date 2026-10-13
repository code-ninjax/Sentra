# Sentra

**Containers, minus the daemon.**

Sentra is a lightweight, daemonless, OCI-compliant container runtime written
in pure Go (`CGO_ENABLED=0`). It pulls and runs Docker Hub / GHCR images,
builds them from a `Sentrafile`, and pushes them back — without a background
service, a socket, or a daemon process to babysit.

The CLI forks directly into Linux namespaces the same way `podman` does, wraps
`runc` for the actual container start, and stores everything as plain files
under `/var/lib/sentra` (or `~/.sentra` when rootless).

---

## Quick start

```sh
# build from source (Go 1.25)
CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o sentra ./cmd/sentra

# pull an OCI image and get a merged rootfs
sudo ./sentra pull alpine:latest

# run a command in that rootfs
sudo ./sentra run --rootfs /var/lib/sentra/rootfs/alpine-latest -- /bin/echo hello

# or build from a Sentrafile and run the resulting tag
sudo ./sentra build -t myapp .
sudo ./sentra run -t myapp
```

See [docs/quickstart.md](docs/quickstart.md) for the full pull → build → run →
push workflow.

---

## Commands

| Command | What it does |
|---|---|
| `sentra pull IMAGE` | Fetch an OCI image and build a merged rootfs |
| `sentra build -t TAG [CONTEXT]` | Run a `Sentrafile` and produce a runnable image |
| `sentra run --rootfs DIR \| -t TAG -- CMD` | Run a command in a container |
| `sentra push TAG [DEST]` | Upload a built image to a registry |
| `sentra images [-r TAG]` | List or remove built image tags |
| `sentra ps [-a]` | List containers |
| `sentra stop CONTAINER` | Stop a running container |
| `sentra rm [-f] CONTAINER` | Remove a container |
| `sentra logs CONTAINER [-f]` | Show container logs |
| `sentra exec CONTAINER -- CMD` | Run a command inside a running container |
| `sentra config FILE` | Parse a `Sentrafile` and print the build plan |

### `run` flags

```
--rootfs DIR     path to an unpacked rootfs directory
-t TAG           run a tag from a previous `sentra build -t TAG`
--name ID        container ID (default: random hex)
--memory MB      memory limit in MiB (0 = unlimited)
--cpus N         CPU cores limit, e.g. 1.5 (0 = unlimited)
-d               run detached in the background
--rw             writable rootfs (read-only is the default)
--security LVL   seccomp level: default, strict or unconfined
--rootless       run in a user namespace (on by default)
-e K=V           extra environment (repeatable, comma-separated)
-p SPEC          publish ports: 8080, 8080:80, 8080:80/udp
--no-publish     do not publish the ports a built image exposed
```

### `build` flags

```
-t TAG           image tag (required)
--no-cache       ignore cached layers and re-run every step
--workers N      parallel step workers (0 = auto, 2-4)
--file PATH      path to the Sentrafile (default: CONTEXT/Sentrafile)
```

---

## The Sentrafile

A `Sentrafile` is **not** a Dockerfile. It is a small, line-oriented format
with no shell and no per-instruction flags:

```
base alpine:3.20
workdir /app
copy . .
exec apk add --no-cache ca-certificates
env GIN_MODE=release
expose 8080/tcp
start /app/myapp
```

Directives:

| Directive | Purpose |
|---|---|
| `base <image> [as <name>]` | Base image; starts a stage. Required, first line. |
| `workdir <abs-path>` | Working directory for subsequent steps. |
| `copy [--from=<stage>] <src> <dest>` | Copy files from the build context (or an earlier stage). |
| `exec <cmd> [args...]` | Run a command during the build (argv form, no implicit shell). |
| `env KEY=VALUE ...` | Set environment variables. |
| `expose <port>[/proto] ...` | Declare container ports (metadata; auto-published on `run`). |
| `start <cmd> [args...]` | Container entrypoint (at most one). |
| `cache key=<value>` | Manual cache-key override for the next step. |
| `secure rootless=<bool> readonly=<bool> seccomp=<default\|strict>` | Declare the image's security posture. |

See [docs/sentrafile.md](docs/sentrafile.md) for the full syntax reference,
including multi-stage builds, `cache`, and `secure`.

---

## Security by default

Sentra's baseline posture is:

- **Rootless** — runs in a user namespace when invoked as non-root.
- **Read-only rootfs** — a container that needs to write must declare it
  (`--rw` at runtime, or writable paths in a future release).
- **Seccomp** — a broad allowlist (`default`) unless the `Sentrafile` asks for
  `strict` or the user passes `--security unconfined`.

The posture is inherited: a `Sentrafile`'s `secure` directive is recorded at
build time and picked up automatically by `sentra run -t`, so the image's
security travels with it.

---

## Networking

One bridge (`sentra0`), one subnet (`10.89.0.0/24`), outbound NAT, and
published ports via `iptables`. No driver zoo. Rootless containers get
loopback only — run with `sudo` for outbound networking and `-p`.

---

## Building from source

```sh
# quick dev build
go build -o sentra ./cmd/sentra

# release build (stripped, reproducible)
bash scripts/build.sh

# release build with UPX compression
bash scripts/build.sh --pack
```

The release build enforces the project's binary-size budget: ≤ 15 MiB
unpacked, ≤ 6 MiB UPX-packed.

---

## Project layout

```
sentra/
├── cmd/sentra/            # thin CLI entrypoint
├── internal/
│   ├── cli/               # run, build, pull, push, ps, stop, rm, logs, exec
│   ├── config/            # Sentrafile parser
│   ├── build/             # build engine: DAG, cache, executor, push
│   ├── runtime/           # runc wrapping, namespaces, cgroups, state store
│   ├── image/             # OCI pull/push, registry, layer handling
│   ├── overlayfs/         # mount and layer store
│   ├── security/          # rootless, seccomp, masked/readonly paths
│   └── network/           # bridge, port mapping, NAT
├── scripts/build.sh       # reproducible release build
├── docs/                  # Sentrafile syntax, architecture, comparison
└── website/               # landing page
```

---

## Docs

- [Quickstart](docs/quickstart.md)
- [Sentrafile syntax reference](docs/sentrafile.md)
- [Architecture](docs/architecture.md)
- [Sentra vs Docker vs Podman](docs/comparison.md)
- [Contributing](CONTRIBUTING.md)

---

## Status

Core runtime, image handling, Sentrafile format, build engine, security
defaults, networking, binary-size/release pipeline, and landing page are
done. Benchmarks are measured — see below.

---

## Benchmarks

`sentra run` vs `docker run`, same pinned image (`alpine:3.20`), WSL2 4CPU,
runc 1.4.0, Docker 29.1.3. Method: wall-clock of a full CLI run of
`/bin/true` (startup, median of 20 interleaved runs) and VmHWM of a
detached `/bin/sleep` init (memory, median of 5). Raw samples and
methodology: [`benchmarks/results/`](benchmarks/results/),
reproduce with `./scripts/benchmark.sh` inside WSL2.

| metric | sentra | docker |
|---|---|---|
| startup median | **0.39s** | 0.89s |
| idle VmHWM median | 0.99MB | **0.86MB** |

Sentra starts containers ~2.3x faster with no daemon; idle footprint is
within ~15% — the tradeoff is stated, not hidden.
