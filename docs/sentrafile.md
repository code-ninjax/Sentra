# Sentrafile syntax reference

A `Sentrafile` is Sentra's build configuration format. It is intentionally
**not** Dockerfile-compatible: a small, line-oriented format with no shell and
no per-instruction flags. Every directive is a bare keyword followed by simple
arguments.

```
# comment
base <image> [as <name>]
workdir <abs-path>
copy [--from=<stage>] <src> <dest>
exec <cmd> [args...]
env KEY=VALUE [KEY=VALUE...]
expose <port>[/proto] ...
start <cmd> [args...]
cache key=<value>
secure rootless=<bool> readonly=<bool> seccomp=<default|strict>
```

- Blank lines are ignored.
- Directives are case-insensitive (`BASE` == `base`).
- Comments are full-line only; a `#` mid-line is literal.
- Arguments use shell-like quoting: single quotes are literal, double quotes
  honor backslash escapes, and a bare backslash escapes the next character.
- There is **no** variable expansion, globbing, or command substitution.

---

## Directives

### `base <image> [as <name>]`

The base image for a build stage. **Required**, and must be the first
directive in the file. Each `base` line starts a new stage.

```
base alpine:3.20
base golang:1.22 as builder
```

- `<image>` is any OCI image reference Sentra can pull from Docker Hub or
  GHCR (e.g. `alpine`, `alpine:3.20`, `ghcr.io/org/img:tag`).
- `as <name>` names the stage so a later stage can reference it with
  `base <name>` or `copy --from=<name>`. Stage names may contain letters,
  digits, dashes, and underscores only.
- Every stage **except the last** must be named — an unnamed stage cannot be
  referenced, so it would be built and thrown away.
- The final stage produces the image.

### `workdir <abs-path>`

Sets the working directory for subsequent `copy` destinations and `exec`
commands. Must be an absolute path.

```
workdir /app
```

The directory is created if it does not exist, and that creation is captured
in the step's layer.

### `copy [--from=<stage>] <src> <dest>`

Copies files or directories. By default `<src>` is read from the build
context; with `--from=<stage>` it is read from an earlier stage's merged
rootfs.

```
copy . .
copy --from=builder /out/server /usr/local/bin/server
```

- `<src>` and `<dest>` are the only positional arguments.
- `--from=` must be a stage declared earlier in the file.
- `<dest>` is resolved relative to the current `workdir` unless it is
  absolute.
- Copy steps are content-addressed and cached automatically — a rebuild only
  re-runs a `copy` when the files it reads change.

### `exec <cmd> [args...]`

Runs a command during the build. Arguments are passed **as argv** — there is
no implicit shell, so `exec ls -l /tmp` runs `ls` with `[-l, /tmp]`, not a
shell parsing a string.

```
exec apk add --no-cache ca-certificates
exec go build -o /app/server ./cmd/server
```

- `exec` steps are **not** automatically cached, because a command's result
  depends on the world, not just its argv. To cache an `exec` step, precede it
  with a `cache key=` directive.

### `env KEY=VALUE [KEY=VALUE...]`

Sets one or more environment variables, visible to subsequent `exec` steps
and recorded in the image config.

```
env GIN_MODE=release GOPATH=/go PATH=/go/bin:$PATH
```

Later assignments for the same key win.

### `expose <port>[/proto] ...`

Declares container ports as metadata. Ports are automatically published on
`sentra run` unless `--no-publish` is passed.

```
expose 8080
expose 8080/tcp 53/udp
```

- `<port>` is 1–65535.
- `/proto` is one of `tcp`, `udp`, or `sctp` (default `tcp`).

### `start <cmd> [args...]`

The container's entrypoint. At most one `start` is allowed. If omitted, the
user must supply a command to `sentra run`.

```
start /app/server
start /bin/sh -c "exec /app/server"
```

Like `exec`, arguments are argv form — no implicit shell.

### `cache key=<value>`

Attaches a manual cache key to the **next** step. This overrides Sentra's
automatic content-addressed caching and is the escape hatch for steps whose
inputs are not just files (typically `exec`).

```
cache key=apt-v1
exec apt-get update && apt-get install -y curl
```

- A `cache` directive with no step beneath it is a parse error.
- Two consecutive `cache` directives are a parse error.
- The manual key bypasses input hashing and the parent layer entirely — it is
  a promise by the author that this step's output can be reused under this
  identity.

### `secure rootless=<bool> readonly=<bool> seccomp=<default|strict>`

Declares the image's security posture. At most one `secure` directive is
allowed, anywhere in the file. The posture is recorded at build time and
applied automatically by `sentra run -t`.

```
secure rootless=true readonly=true seccomp=strict
```

| Option | Values | Default |
|---|---|---|
| `rootless` | `true`, `false` | `true` |
| `readonly` | `true`, `false` | `true` |
| `seccomp` | `default`, `strict` | `default` |

- `rootless=true`: run in a user namespace owned by the invoking user.
- `readonly=true`: mount the rootfs read-only.
- `seccomp=default`: broad allowlist for ordinary workloads.
- `seccomp=strict`: default minus syscalls that reach into kernel, module, or
  device internals — for images that do not need them.

The defaults match Sentra's runtime baseline, so omitting `secure` gives you
the secure-by-default posture.

---

## Multi-stage builds

A `Sentrafile` may declare several stages. Only the final stage becomes the
image. Earlier stages exist to feed `copy --from=` or to serve as the base of
a later stage.

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
start /usr/local/bin/server
```

- Each stage must be named except the last.
- `base <name>` starts a stage from an earlier stage's full layer chain —
  free, no re-pull.
- `copy --from=<name>` lifts files out of an earlier stage's merged rootfs.

---

## Inspecting a Sentrafile

`sentra config` parses a `Sentrafile` and prints the resulting build plan
without executing it:

```
sentra config Sentrafile
sentra config -q Sentrafile   # validate only, print "ok"
```

Example output:

```
stages  2
secure  rootless=true readonly=true seccomp=default

stage   builder   base golang:1.22
  steps 5
  3 base golang:1.22
  4 workdir /src
  ...

image   (final)   base alpine:3.20
  steps 3
  ...
```
