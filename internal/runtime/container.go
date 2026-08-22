package runtime

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/opencontainers/runtime-spec/specs-go"
)

// Container is Sentra's record of a container, persisted as JSON under the
// state dir. It is the single source of truth for `sentra ps/rm/logs/stop`.
type Container struct {
	ID        string    `json:"id"`
	Image     string    `json:"image,omitempty"`
	Rootfs    string    `json:"rootfs"`
	Cmd       []string  `json:"cmd"`
	Status    string    `json:"status"` // creating|running|exited|stopped
	Pid       int       `json:"pid"`
	ExitCode  int       `json:"exit_code"`
	CreatedAt time.Time `json:"created_at"`
	Bundle    string    `json:"bundle"`
	LogFile   string    `json:"log_file"`
	Readonly  bool      `json:"readonly"`
	Detached  bool      `json:"detached"`
}

// Seccomp is intentionally absent from the spec builder below.
// TODO(workstream 5): install a default seccomp deny-by-allowlist profile
// on spec.Linux.Seccomp and add MaskedPaths/ReadonlyPaths hardening.

// ---------------------------------------------------------------------------
// Spec building
// ---------------------------------------------------------------------------

// BuildSpec produces the OCI runtime-spec config.json contents for a
// container: namespaces (namespace.go), cgroups v2 limits (cgroups.go),
// read-only root by default, and the standard /proc /dev /sys mounts runc
// requires us to declare explicitly.
func BuildSpec(c *Container, limits CgroupLimits) (*specs.Spec, error) {
	rootless := os.Getuid() != 0

	rootfsAbs, err := filepath.Abs(c.Rootfs)
	if err != nil {
		return nil, fmt.Errorf("resolve rootfs path: %w", err)
	}

	host := c.ID
	if len(host) > 12 {
		host = host[:12]
	}

	spec := &specs.Spec{
		Version: "1.1.0",
		Process: &specs.Process{
			Terminal: false,
			User:     specs.User{},
			Args:     c.Cmd,
			Env: []string{
				"PATH=/usr/local/sbin:/usr/local/bin:/usr/sbin:/usr/bin:/sbin:/bin",
				"TERM=xterm",
				"HOME=/root",
			},
			Cwd: "/",
		},
		Hostname: host,
		Root: &specs.Root{
			Path:     rootfsAbs,
			Readonly: c.Readonly,
		},
		Linux: &specs.Linux{
			Namespaces: DefaultNamespaces(rootless),
			Resources:  limits.Resources(),
			// TODO(workstream 5): Seccomp + MaskedPaths/ReadonlyPaths here.
		},
	}
	spec.Mounts = DefaultMounts(rootless)
	return spec, nil
}

// DefaultMounts declares the pseudo-filesystems every container needs.
// runc mounts nothing implicitly — an undeclared /proc means no /proc.
func DefaultMounts(rootless bool) []specs.Mount {
	mounts := []specs.Mount{
		{Destination: "/proc", Type: "proc", Source: "proc"},
		{Destination: "/dev", Type: "tmpfs", Source: "tmpfs",
			Options: []string{"nosuid", "strictatime", "mode=755", "size=65536k"}},
		{Destination: "/dev/pts", Type: "devpts", Source: "devpts",
			Options: []string{"nosuid", "noexec", "newinstance", "ptmxmode=0666", "mode=0620"}},
		{Destination: "/dev/shm", Type: "tmpfs", Source: "shm",
			Options: []string{"nosuid", "noexec", "nodev", "mode=1777", "size=65536k"}},
		{Destination: "/sys", Type: "sysfs", Source: "sysfs",
			Options: []string{"nosuid", "noexec", "nodev", "ro"}},
		{Destination: "/tmp", Type: "tmpfs", Source: "tmpfs",
			Options: []string{"nosuid", "noexec", "nodev", "mode=1777"}},
	}
	if rootless {
		// Mounting proc/sysfs fresh inside a user namespace is restricted
		// on many kernels; bind-mounting the host's copies always works.
		mounts[0] = specs.Mount{Destination: "/proc", Type: "bind",
			Source: "/proc", Options: []string{"nosuid", "noexec", "nodev"}}
		mounts[4] = specs.Mount{Destination: "/sys", Type: "bind",
			Source: "/sys", Options: []string{"nosuid", "noexec", "nodev", "ro"}}
	}
	return mounts
}

// CreateBundle writes the runc bundle for c: <bundles>/<id>/config.json.
// The bundle dir is what we hand to `runc run/create --bundle`.
func CreateBundle(c *Container, limits CgroupLimits) error {
	bundlesDir, err := ensureDir("bundles")
	if err != nil {
		return err
	}
	bundle := filepath.Join(bundlesDir, c.ID)
	if err := os.MkdirAll(bundle, 0o755); err != nil {
		return fmt.Errorf("create bundle dir: %w", err)
	}

	spec, err := BuildSpec(c, limits)
	if err != nil {
		return err
	}
	data, err := json.MarshalIndent(spec, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal config.json: %w", err)
	}
	cfg := filepath.Join(bundle, "config.json")
	if err := os.WriteFile(cfg, data, 0o644); err != nil {
		return fmt.Errorf("write config.json: %w", err)
	}
	c.Bundle = bundle
	c.LogFile = filepath.Join(mustLogsDir(), c.ID+".log")
	return nil
}

// ---------------------------------------------------------------------------
// State store — one JSON file per container
// ---------------------------------------------------------------------------

func SaveContainer(c *Container) error {
	dir, err := ensureDir("containers")
	if err != nil {
		return err
	}
	data, err := json.MarshalIndent(c, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal container state: %w", err)
	}
	path := filepath.Join(dir, c.ID+".json")
	return os.WriteFile(path, data, 0o644)
}

func LoadContainer(id string) (*Container, error) {
	dir, err := stateDir()
	if err != nil {
		return nil, err
	}
	data, err := os.ReadFile(filepath.Join(dir, "containers", id+".json"))
	if err != nil {
		return nil, fmt.Errorf("container %q not found", id)
	}
	c := &Container{}
	if err := json.Unmarshal(data, c); err != nil {
		return nil, fmt.Errorf("corrupt state for %q: %w", id, err)
	}
	return c, nil
}

func ListContainers() ([]*Container, error) {
	dir, err := ensureDir("containers")
	if err != nil {
		return nil, err
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, err
	}
	var out []*Container
	for _, e := range entries {
		if e.IsDir() || filepath.Ext(e.Name()) != ".json" {
			continue
		}
		data, err := os.ReadFile(filepath.Join(dir, e.Name()))
		if err != nil {
			continue // skip unreadable entries rather than failing ps entirely
		}
		c := &Container{}
		if json.Unmarshal(data, c) == nil {
			out = append(out, c)
		}
	}
	return out, nil
}

// RemoveContainer deletes the state record and the runc bundle directory.
// The log file is kept so `sentra logs` still works after rm until workstream
// 7 adds retention policy.
func RemoveContainer(id string) error {
	dir, err := stateDir()
	if err != nil {
		return err
	}
	if err := os.Remove(filepath.Join(dir, "containers", id+".json")); err != nil && !os.IsNotExist(err) {
		return err
	}
	if err := os.RemoveAll(filepath.Join(dir, "bundles", id)); err != nil {
		return err
	}
	return nil
}

// NewID returns a short random hex container ID.
func NewID() (string, error) {
	b := make([]byte, 6)
	if _, err := rand.Read(b); err != nil {
		return "", fmt.Errorf("generate container id: %w", err)
	}
	return hex.EncodeToString(b), nil
}

// ---------------------------------------------------------------------------
// State dir resolution
// ---------------------------------------------------------------------------

var cachedStateDir string

// stateDir returns the Sentra state root. Prefers /var/lib/sentra when
// writable (root), falls back to ~/.sentra (rootless). No daemon, no lock
// server — plain files.
func stateDir() (string, error) {
	if cachedStateDir != "" {
		return cachedStateDir, nil
	}
	for _, dir := range []string{"/var/lib/sentra"} {
		if ok := tryWrite(dir); ok {
			cachedStateDir = dir
			return dir, nil
		}
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("no writable state dir: %w", err)
	}
	dir := filepath.Join(home, ".sentra")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", err
	}
	cachedStateDir = dir
	return dir, nil
}

func tryWrite(dir string) bool {
	if os.MkdirAll(dir, 0o755) != nil {
		return false
	}
	probe := filepath.Join(dir, ".probe")
	f, err := os.Create(probe)
	if err != nil {
		return false
	}
	f.Close()
	os.Remove(probe)
	return true
}

func ensureDir(sub string) (string, error) {
	dir, err := stateDir()
	if err != nil {
		return "", err
	}
	full := filepath.Join(dir, sub)
	if err := os.MkdirAll(full, 0o755); err != nil {
		return "", err
	}
	return full, nil
}

func mustLogsDir() string {
	dir, err := ensureDir("logs")
	if err != nil {
		return os.TempDir()
	}
	return dir
}
