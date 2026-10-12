# Quickstart

This guide walks through Sentra's core workflow: pull an image, build from a
`Sentrafile`, run it, push it, and manage containers.

## Prerequisites

- **Linux** with `runc` on `PATH` (Sentra wraps `runc` for container starts).
- **Go 1.25** to build from source.
- **Root** (or `sudo`) for outbound networking and port publishing — rootless
  runs get loopback only.
- `ip` and `iptables` on `PATH` for networking.

## Build Sentra

```sh
git clone <repo>
cd sentra

# dev build
go build -o sentra ./cmd/sentra

# release build (stripped, reproducible, size-budget gated)
bash scripts/build.sh
# → dist/sentra-linux-amd64

# release build with UPX compression
bash scripts/build.sh --pack
# → dist/sentra-linux-amd64-packed
```

## 1. Pull an image

`sentra pull` fetches an OCI image from Docker Hub or GHCR, extracts its
layers, and merges them into a rootfs directory.

```sh
sudo ./sentra pull alpine:latest
```

Output:

```
pulling docker.io/library/alpine:latest@sha256:...
  layer 1/1 fetching ... (xxx bytes)...
rootfs ready: /var/lib/sentra/rootfs/alpine-latest
```

The merged rootfs is at the printed path, ready for `sentra run --rootfs`.

## 2. Run a container

Run a one-off command against the pulled rootfs:

```sh
sudo ./sentra run --rootfs /var/lib/sentra/rootfs/alpine-latest -- /bin/echo hello
```

Sentra reports the security posture on stderr:

```
sentra: rootless, read-only rootfs, seccomp=default
hello
```

### Common `run` flags

```sh
# detached background container
sudo ./sentra run -d --rootfs /var/lib/sentra/rootfs/alpine-latest -- /bin/sleep 60

# name it, limit memory and CPU
sudo ./sentra run --name web --memory 512 --cpus 1.5 -t myapp

# writable rootfs
sudo ./sentra run --rw -t myapp -- /bin/sh

# extra env vars
sudo ./sentra run -e FOO=bar,BAZ=qux -t myapp

# publish ports (8080 on host → 80 in container)
sudo ./sentra run -p 8080:80 -t myapp

# stricter seccomp
sudo ./sentra run --security strict -t myapp
```

Run a built image by tag:

```sh
sudo ./sentra run -t myapp
```

## 3. Build from a Sentrafile

Create a `Sentrafile`:

```
base golang:1.22 as builder
workdir /src
copy go.mod go.sum .
exec go mod download
copy . .
exec go build -o /out/server ./cmd/server

base alpine:3.20
exec apk add --no-cache ca-certificates
copy --from=builder /out/server /usr/local/bin/server
expose 8080
start /usr/local/bin/server
```

Build it:

```sh
sudo ./sentra build -t myapp .
```

Output:

```
base golang:1.22
[0] base    golang:1.22
[1] workdir /src
[2] copy    go.mod -> .
[3] exec    go mod download
...
built myapp in 4.2s (2/5 layers cached)
rootfs: /var/lib/sentra/built/myapp-rootfs
run it:
  sudo ./sentra run --rootfs /var/lib/sentra/built/myapp-rootfs -- /usr/local/bin/server
exposes: 8080 (host publishing arrives with the networking workstream)
```

Build flags:

```sh
# force rebuild, skip cache
sudo ./sentra build -t myapp --no-cache .

# custom Sentrafile path
sudo ./sentra build -t myapp --file ./build/Sentrafile .

# control parallel workers
sudo ./sentra build -t myapp --workers 2 .
```

## 4. Push to a registry

Push a built image. Only the build's own layers are uploaded — the base image
is referenced from the registry it came from.

```sh
# push with the same tag (targets docker.io/library/myapp)
sudo ./sentra push myapp

# push to a different destination
sudo ./sentra push myapp ghcr.io/myorg/myapp:v1
```

For registries requiring auth, set `SENTRA_REGISTRY_TOKEN`:

```sh
SENTRA_REGISTRY_TOKEN=$(cat token.txt) sudo ./sentra push myapp ghcr.io/myorg/myapp:v1
```

Output:

```
pushed ghcr.io/myorg/myapp:v1
digest sha256:... (2 build layer(s))
```

> Images built without overlayfs layer support (e.g. non-root builds that fell
> back to a plain rootfs copy) cannot be pushed. Rebuild as root
> (`sudo sentra build`) for a pushable image.

## 5. Manage containers

```sh
# list containers
sudo ./sentra ps

# stop a running container
sudo ./sentra stop <id>

# remove a container (use -f for running ones)
sudo ./sentra rm <id>
sudo ./sentra rm -f <id>

# view logs (follow with -f)
sudo ./sentra logs <id>
sudo ./sentra logs -f <id>

# run a command inside a running container
sudo ./sentra exec <id> -- /bin/sh
```

## 6. Manage built images

```sh
# list built image tags
sudo ./sentra images

# remove a built image tag
sudo ./sentra images -r myapp
```

## 7. Inspect a Sentrafile

Validate a `Sentrafile` and see the build plan without executing it:

```sh
./sentra config Sentrafile
./sentra config -q Sentrafile   # validate only, prints "ok"
```

## State directory

Sentra stores everything as plain files:

| Path (root) | Path (rootless) | Contents |
|---|---|---|
| `/var/lib/sentra` | `~/.sentra` | all state |
| `…/blobs` | | downloaded OCI layers (content-addressed) |
| `…/layers` | | extracted layer directories |
| `…/rootfs` | | merged rootfs from `pull` |
| `…/built` | | built image records + rootfs |
| `…/build-cache` | | per-step layer diff cache |
| `…/containers` | | container state (one JSON per container) |
| `…/bundles` | | runc bundles |
| `…/logs` | | container stdout/stderr logs |
| `…/network` | | bridge address allocation table |

No daemon, no socket, no lock server.
