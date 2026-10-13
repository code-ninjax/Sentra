// Site-wide config. Edit only this file for links, install and numbers.
export const CONFIG = {
  GITHUB_URL: "https://github.com/code-ninjax/Sentra",
  GITHUB_REPO: "code-ninjax/Sentra",
  DOCS_URL: "#/docs",
  INSTALL_CMD:
    "curl -fsSLO https://github.com/code-ninjax/Sentra/releases/latest/download/sentra-linux-amd64 && chmod +x sentra-linux-amd64 && sudo mv sentra-linux-amd64 /usr/local/bin/sentra",
  RELEASES_URL: "https://github.com/code-ninjax/Sentra/releases",
  // Real medians from benchmarks/results (2026-10-13, WSL2 4CPU).
  // Zeros render as "TBD" ghost bars — docker side lands when the
  // Docker Desktop WSL integration is on; rerun scripts/benchmark.sh.
  benchmarks: {
    startup: { sentra: 0.39, docker: 0, unit: "s", n: 20, stamp: "2026-10-13" },
    memory: { sentra: 1.04, docker: 0, unit: "MB", n: 5, stamp: "2026-10-13" },
  },
};

export const FACES = [
  {
    title: "Daemonless",
    desc: "The CLI execs straight into namespaces. No background process, no socket, nothing idle between your commands.",
    stat: "sentra run --rootfs ./rootfs -- /bin/echo hi",
    icon: "layers",
  },
  {
    title: "Fast builds",
    desc: "Direct overlayfs layers, copy_file_range zero-copy, content-addressed cache, and a parallel DAG scheduler.",
    stat: "3/3 layers cached · 7.9s to 5.06s parallel",
    icon: "zap",
  },
  {
    title: "Secure by default",
    desc: "Rootless, read-only rootfs, and seccomp profiles — declared in the Sentrafile, enforced at runtime.",
    stat: "sentra run --security strict -t myapp",
    icon: "shield",
  },
  {
    title: "OCI-native",
    desc: "Pulls and pushes Docker Hub and GHCR images. Runs any existing image; Docker can pull what Sentra pushes.",
    stat: "sentra pull alpine:3.20",
    icon: "box",
  },
  {
    title: "Tiny binary",
    desc: "Pure Go, CGO off. One static binary that does the work and exits — 8MB unpacked, ~3MB packed.",
    stat: "8.29 MB unpacked · 3.09 MB UPX",
    icon: "feather",
  },
  {
    title: "Open source",
    desc: "Apache 2.0, built in public. Plain directories for layers, a cache you can read, a format with 8 directives.",
    stat: "github.com/code-ninjax/Sentra",
    icon: "sparkle",
  },
];

export const STACKS = [
  { name: "Node.js", blurb: "Any node:20-slim image runs unmodified.", glyph: "node" },
  { name: "Python", blurb: "python:3.12-slim and friends, straight from Hub.", glyph: "python" },
  { name: "Go", blurb: "Compile in golang, ship a static binary on alpine.", glyph: "go" },
  { name: "Rust", blurb: "Multi-stage builds keep toolchains out of prod.", glyph: "rust" },
  { name: "Docker", blurb: "Speaks the registry protocol Docker speaks.", glyph: "docker" },
  { name: "Postgres", blurb: "Stateful images run; data lives in volumes.", glyph: "db" },
  { name: "Redis", blurb: "Publish a port, talk to it from the host.", glyph: "redis" },
  { name: "Bun", blurb: "Any OCI base works — no per-language logic.", glyph: "bun" },
  { name: "Express", blurb: "Framework images are just files plus a command.", glyph: "express" },
];

export const ROADMAP = [
  {
    title: "CRIU snapshot/restore",
    desc: "Checkpoint a running container and restore it later, for fast restarts and migration.",
    icon: "camera",
  },
  {
    title: "eBPF auto-seccomp",
    desc: "Learn a workload's real syscalls and generate a tight profile automatically.",
    icon: "activity",
  },
  {
    title: "AI diagnostics",
    desc: "sentra fix, optimize and ask for build and runtime help.",
    icon: "sparkle",
  },
  {
    title: "--distroless",
    desc: "Minimal runtime images synthesized at build time, without a base distro.",
    icon: "box",
  },
  {
    title: "Desktop app",
    desc: "A Mac and Windows GUI over the same binary. Post-MVP — separate from the core MVP.",
    icon: "monitor",
  },
];
