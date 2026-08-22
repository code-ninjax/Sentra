# STEP 1 — Core Runtime

Paste this into a fresh chat with your AI (after it has read AGENT.md /
project instructions for Sentra).

---

## Context

I'm building Sentra, a daemonless Go container runtime. Read the project
rules first (Go only, wrap runc, cgroups v2, overlayfs, no daemon, binary
size matters). This chat is **Step 1 of 10: Core Runtime**.

## Scope for this chat

Files in scope:
```
cmd/sentra/main.go
internal/cli/run.go
internal/cli/ps.go
internal/cli/stop.go
internal/cli/rm.go
internal/cli/logs.go
internal/cli/exec.go
internal/runtime/container.go
internal/runtime/namespace.go
internal/runtime/cgroups.go
internal/runtime/runc.go
```
(`internal/cli/build.go` and everything under `internal/build/` is Step 4 —
out of scope here.)

## Goal

Get `sentra run <image>` working end-to-end: given an already-unpacked
rootfs on disk (assume Step 2's image layer has produced one — don't build
image pulling here), fork/exec into a runc-managed container with proper
namespace isolation (pid, net, mnt, uts, ipc) and cgroups v2 limits, run the
image's entrypoint, stream output, and exit cleanly.

## Build order within this chat

1. **Scaffold `cmd/sentra/main.go`** — thin entrypoint, wires up cobra (or
   stdlib flag) subcommands from `internal/cli`. No logic here.
2. **`internal/runtime/runc.go`** — wrapper around invoking `runc` (either
   shelling out to the `runc` binary or using its Go API/libcontainer,
   whichever is more maintainable — state your choice and why).
3. **`internal/runtime/namespace.go`** — namespace configuration
   (pid/net/mnt/uts/ipc) for the OCI runtime spec (`config.json`) runc needs.
4. **`internal/runtime/cgroups.go`** — cgroups v2 resource limit wiring
   (start minimal: memory + cpu, extensible later).
5. **`internal/runtime/container.go`** — ties the above together: build OCI
   runtime spec, create container via runc, start it, wait for exit, capture
   stdout/stderr.
6. **`internal/cli/run.go`** — `sentra run <image>` command. For this chat,
   accept a path to an already-unpacked rootfs directory instead of pulling
   a real image (stub the image-fetch part with a `--rootfs` flag or a
   hardcoded test dir) — the real OCI pull is Step 2.
7. **Stub the rest**: `ps.go`, `stop.go`, `rm.go`, `logs.go`, `exec.go` —
   get them compiling with real command signatures and a minimal in-memory
   or file-based container state store (e.g. JSON in `/var/lib/sentra` or
   similar), even if functionality is bare-bones. Don't fully build these
   out — just don't leave the CLI half-wired.

## Constraints

- Rootless by default — don't require root unless truly unavoidable for a
  given namespace op; call out explicitly anywhere root *is* required.
- Read-only rootfs by default (mount option on the OCI spec).
- No seccomp profile logic yet — that's Step 5 (Security Defaults). Leave a
  clear TODO/interface stub.
- No networking setup beyond what's needed to not crash — real bridge/overlay
  network is Step 6.
- Keep `main.go` under ~40 lines. All real logic lives in `internal/`.

## Deliverable for this chat

- Compiling Go code for all files listed above.
- A working `sentra run <path-to-rootfs>` that: creates the runc bundle,
  starts the container, streams stdout/stderr to the terminal, and exits
  cleanly with the container's exit code.
- Brief note on what's stubbed vs fully implemented, and what Step 5/6 will
  need to come back and fill in.

## Test target

Use a minimal already-extracted rootfs (e.g. busybox or alpine rootfs
untarred to a local directory) to validate `sentra run` prints something
and exits 0. Don't build the OCI pull path to get this test data — that's
explicitly Step 2's job.
