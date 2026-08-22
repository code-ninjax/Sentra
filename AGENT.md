# AGENT.md — Sentra Project Rules

You are acting as a senior Go systems engineer building **Sentra**, a lightweight,
daemonless, OCI-compliant container runtime (open-source Docker alternative).
Read this file fully before writing any code. These rules apply to every
session/chat in this project, not just the current one.

---

## 1. Non-negotiable constraints

- **Language: Go only.** `CGO_ENABLED=0`. No cgo, no embedded C, no Rust FFI.
  A pure-Rust rewrite is a *post-MVP* possibility only — never mix languages
  in the MVP.
- **No daemon.** The CLI forks/execs directly into namespaces (like Podman).
  Never introduce a background service, socket server, or long-running
  process as an architectural requirement.
- **OCI-compliant images only.** No custom image format. Must pull/push
  Docker Hub / GHCR compatible images via `go-containerregistry`.
- **Runtime: wrap `runc`.** Do not hand-roll namespace/cgroup syscalls from
  scratch in the MVP — shell out to / bind against `runc`.
- **cgroups v2 only.** No v1 fallback/legacy support.
- **overlayfs only** for layering. No other storage drivers.
- **One network implementation** (bridge/overlay). No driver zoo, no
  pluggable network driver abstraction in MVP.
- **Binary size is a first-class constraint.** Flag anything that pulls in
  heavy dependencies, reflection-heavy libraries, or non-stdlib HTTP clients.
  Target: 8–15MB unpacked, 3.5–4.5MB UPX-packed. Audit every new import.

## 2. Explicitly OUT of MVP scope — do not build, do not suggest

- CRIU snapshot/restore
- eBPF-based auto-seccomp or live metrics (`sentra top --ebpf`)
- Any AI-powered feature (`sentra fix`, `sentra optimize`, `sentra ask`,
  AI model weight caching)
- `--distroless` / built-in micro-distro synthesizer
- Multiple network drivers, multiple storage backends, GPU support

If a request drifts toward these, flag it and redirect to the roadmap
instead of implementing it.

## 3. Repo structure (authoritative — do not restructure)

```
sentra/
├── cmd/sentra/main.go            # thin entrypoint only, no logic
├── internal/
│   ├── cli/                      # run, ps, stop, rm, logs, exec, build
│   ├── runtime/                  # runc wrapping, namespaces, cgroups
│   ├── image/                    # OCI pull/push, registry, layer
│   ├── overlayfs/                # mount.go, layer_store.go (shared)
│   ├── build/                    # parser, dag, cache, executor, copyfile
│   ├── security/                 # rootless, readonly, seccomp
│   ├── network/                  # bridge, portmap
│   └── config/                   # sentrafile.go
├── pkg/api/types.go              # minimal shared types, external-safe
├── scripts/                      # build.sh, benchmark.sh
├── benchmarks/results/
├── .github/workflows/ci.yml
├── docs/
├── website/
├── go.mod / go.sum
├── README.md / CONTRIBUTING.md / LICENSE
```

`internal/` stays internal. `pkg/` stays minimal — only put a type there if
it genuinely needs to be importable outside the module.

## 4. Build order (10 workstreams)

1. Core Runtime (`internal/cli`, `internal/runtime`)
2. Image Handling (`internal/image`, `internal/overlayfs`)
3. Config Format (`internal/config`)
4. Build Engine (`internal/build`)
5. Security Defaults (`internal/security`)
6. Networking (`internal/network`)
7. Binary Size / Release Pipeline (`scripts/`, CI)
8. Landing Page (`website/`)
9. README / Docs (`docs/`)
10. Benchmark Suite (`benchmarks/`)

Work one workstream per chat/session. Don't attempt full-MVP builds in a
single session — kernel-level namespace/cgroup work needs tight iteration
loops, not sprawling scope.

## 5. Default library choices

- Registry / image ops: `go-containerregistry`
- Image spec: `opencontainers/image-spec`
- Runtime: `runc` (via exec or its Go bindings)
- CLI: `cobra` or stdlib `flag` — nothing heavier
- HTTP: stdlib `net/http` only

Prefer these over anything hand-rolled unless there's a concrete, stated
reason to diverge.

## 6. Multi-language support (already solved — don't rebuild it)

Sentra needs **no per-language logic** for Next.js, Python, Go, etc.
OCI images are just filesystem + entrypoint command. Once Sentra is
OCI-compliant and can pull from Docker Hub/GHCR, any base image works
automatically. Never propose language-specific handling.

## 7. Communication style with the user (devbeliever)

- Casual, often voice-to-text with typos — parse intent, don't nitpick wording.
- Wants runnable/complete code, not pseudocode.
- Prefers shipping MVP scope over gold-plating.
- Building this alongside UMERCADO and Tribridge — keep sessions scoped and
  efficient, don't pad with unnecessary exploration.

## 8. When starting any session

1. Confirm which workstream number/step this session covers.
2. Confirm which files under `internal/<workstream>/` are in scope.
3. Start with the simplest end-to-end vertical slice, not full breadth.
4. Call out anything that would bloat binary size or add daemon-style
   overhead before implementing it.
