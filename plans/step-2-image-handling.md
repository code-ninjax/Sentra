# STEP 2 — Image Handling

Paste this into a fresh chat with your AI (after it has read AGENT.md /
project instructions for Sentra).

---

## Context

I'm building Sentra, a daemonless Go container runtime. Read the project
rules first (Go only, OCI-compliant, overlayfs, no daemon, binary size
matters). This chat is **Step 2 of 10: Image Handling**.

## Scope for this chat

Files in scope:
```
internal/image/pull.go
internal/image/push.go
internal/image/registry.go
internal/image/layer.go
internal/overlayfs/mount.go
internal/overlayfs/layer_store.go
```
(`internal/runtime/*` from Step 1 is done — this chat should produce a
rootfs directory that Step 1's `sentra run --rootfs <path>` can already
consume. `internal/build/*` — the build engine with copy_file_range and
DAG execution — is Step 4, out of scope here.)

## Goal

Get `sentra pull <image>` working end-to-end: fetch an OCI image manifest +
layers from a real registry (Docker Hub and GHCR), verify/store them
content-addressably, and unpack them into overlayfs-compatible layer
directories on disk — ready for Step 1's runtime to mount and run.

## Build order within this chat

1. **`internal/image/registry.go`** — registry client using
   `go-containerregistry` (`pkg/v1/remote`, `pkg/v1/remote/transport`).
   Support Docker Hub and GHCR auth (anonymous pulls first; note where
   authenticated pulls would hook in, don't fully build credential storage
   yet).
2. **`internal/image/pull.go`** — `sentra pull <image>` logic: resolve
   image reference → fetch manifest → fetch config → fetch each layer
   blob. Use `go-containerregistry`'s `v1.Image` interface rather than
   hand-rolling manifest parsing.
3. **`internal/image/layer.go`** — layer representation: digest, size,
   media type, and the extraction step (untar each layer blob into a
   content-addressable directory keyed by layer digest, e.g.
   `~/.sentra/layers/<sha256>/`).
4. **`internal/overlayfs/layer_store.go`** — tracks which layer digests are
   already on disk (skip re-fetching/re-extracting known layers — this is
   the foundation Step 4's build cache will build on, but here it's just
   "don't redo work for `pull`").
5. **`internal/overlayfs/mount.go`** — given an ordered list of layer
   directories (lowerdirs) for an image, construct the overlayfs mount
   (lowerdir/upperdir/workdir) and produce a merged rootfs directory. This
   is what Step 1's `--rootfs` flag should point at.
6. **`internal/image/push.go`** — stub with a clear function signature and
   TODO; don't fully implement push in this chat unless pull is done with
   room to spare. Pull is the priority deliverable.

## Constraints

- Use `go-containerregistry` for all registry/manifest/auth work — don't
  hand-roll HTTP calls to the Docker Registry API.
- Content-addressable storage: key layers and configs by their digest so
  re-pulling an already-known image/layer is a no-op. This also sets up
  Step 4's build-cache requirements later.
- No tar/untar round-trip inside the *build* path — that constraint is for
  Step 4's COPY/ADD instructions specifically. For `pull`, standard
  layer-tar extraction to disk is expected and correct.
- overlayfs only, no other storage driver.
- Keep registry client dependencies minimal — audit anything beyond
  `go-containerregistry` itself for binary size impact.

## Deliverable for this chat

- Compiling Go code for all files listed above.
- A working `sentra pull <image>` (e.g. `sentra pull alpine:latest`) that:
  resolves the reference, downloads manifest + config + layers, extracts
  them into `~/.sentra/layers/<digest>/`, and produces a merged overlayfs
  rootfs directory.
- Confirm interop: the output rootfs path should work directly with Step
  1's `sentra run --rootfs <path>`.
- Brief note on what's stubbed (push, auth) vs fully implemented.

## Test target

`sentra pull alpine:latest` (or busybox) against real Docker Hub, followed
by `sentra run` (from Step 1) against the resulting rootfs, should produce
a running container and clean exit — this is the first true end-to-end
Sentra smoke test.
