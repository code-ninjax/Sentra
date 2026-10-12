# Sentra vs Docker vs Podman

A feature-by-feature comparison based on what Sentra actually ships today.

## Feature matrix

| | Sentra | Docker | Podman |
|---|---|---|---|
| **Daemon** | No — CLI forks into namespaces directly | Yes (`dockerd`) | No — forks into namespaces |
| **OCI image support** | Yes (`go-containerregistry`) | Yes | Yes |
| **Runtime** | Wraps `runc` | Wraps `runc` (via containerd) | Wraps `runc` |
| **Rootless by default** | Yes — user namespace unless root | No (rootful by default; rootless mode exists) | Yes |
| **Read-only rootfs by default** | Yes | No | No |
| **Seccomp by default** | Yes (`default` allowlist) | Yes (Docker default profile) | Yes (containers.conf) |
| **cgroups version** | v2 only | v1 and v2 | v1 and v2 |
| **Storage driver** | overlayfs only | overlayfs, aufs, btrfs, zfs, devicemapper… | overlayfs, native, btrfs, zfs… |
| **Network drivers** | One (bridge + NAT) | bridge, host, overlay, macvlan, none… | bridge, host, macvlan, slirp4netns, pasta… |
| **Rootless networking** | Loopback only (run with `sudo` for bridge) | slirp4netns | slirp4netns, pasta |
| **Build format** | `Sentrafile` (no shell, argv exec) | Dockerfile (shell by default) | Containerfile / Dockerfile |
| **Build cache** | Content-addressed, overlayfs-native (no tar round-trip) | BuildKit layer cache | Buildah layer cache |
| **Multi-stage builds** | Yes (`base … as`, `copy --from=`) | Yes | Yes |
| **Manual cache control** | `cache key=<value>` directive | `--cache-from`, `RUN --mount=type=cache` | Buildah equivalents |
| **Port publishing** | `-p 8080:80`, auto-publishes `expose`d ports | `-p 8080:80`, `expose` is metadata only | `-p 8080:80`, `expose` is metadata only |
| **Image push** | `sentra push TAG [DEST]` | `docker push` | `podman push` |
| **State storage** | Plain JSON files under `/var/lib/sentra` or `~/.sentra` | containerd + SQLite/bolt | boltdb + JSON |
| **Language** | Pure Go (`CGO_ENABLED=0`) | Go (with cgo) | Go (with cgo) |
| **Binary size (target)** | 8–15 MiB unpacked, 3.5–4.5 MiB UPX-packed | ~60 MiB+ (dockerd + CLI) | ~20–30 MiB |

## Design differences

### No daemon, no socket

Sentra runs exactly like `podman`: the CLI process is the container's parent.
There is no `dockerd`-style service, no API socket, no long-running process to
babysit. Every command is a short-lived process that sets up namespaces,
invokes `runc`, and exits.

### Sentrafile is not a Dockerfile

The `Sentrafile` format is deliberately stripped down:

- **No shell.** `exec` and `start` take argv, not a shell string. `exec apk add
  curl` runs `apk` with `[add, curl]` — no `/bin/sh -c` wrapper.
- **No per-instruction flags.** `copy --from=` is the only flag in the format.
  Everything else is positional.
- **No variable expansion, globbing, or command substitution.** Those belong to
  `exec`'s argv, not the parser.

The tradeoff is clarity: a `Sentrafile` is a *build plan*, not a script. The
parser turns it into a flat ordered list of steps and nothing downstream
re-reads the raw text.

### Overlayfs-native caching

Sentra's build cache never tars and untars. The base image's extracted layer
directories are the overlay lowerdirs; each mutating step gets a fresh
upperdir. Whatever the step writes is the diff, and that diff is the cache
entry. A cached `copy` costs one directory lookup, never a filesystem copy.

### Security defaults are strict

Sentra's baseline posture is rootless, read-only rootfs, and seccomp on. A
`Sentrafile`'s `secure` directive is recorded at build time and inherited by
`sentra run -t`, so an image's security travels with it rather than being
re-declared on every run.

### One network, one storage driver

Sentra ships exactly one bridge (`sentra0`, `10.89.0.0/24`) and one storage
driver (overlayfs). There is no driver abstraction, no plugin system, and no
fallback matrix. If a workload needs macvlan or zfs, it does not need Sentra.

## What Sentra does not do (yet, or ever)

Per the project's non-negotiable constraints and roadmap:

- No CRIU snapshot/restore.
- No eBPF-based auto-seccomp or live metrics.
- No AI-powered features.
- No built-in micro-distro synthesizer.
- No GPU support, no multiple network/storage drivers.
- cgroups v1 will not be supported.

These are intentional scope cuts, not missing features.
