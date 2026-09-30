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

	"sentra/internal/security"
)

// Container is Sentra's record of a container, persisted as JSON under the
// state dir. It is the single source of truth for `sentra ps/rm/logs/stop`.
type Container struct {
	ID        string           `json:"id"`
	Image     string           `json:"image,omitempty"`
	Rootfs    string           `json:"rootfs"`
	Cmd       []string         `json:"cmd"`
	Status    string           `json:"status"` // creating|running|exited|stopped
	Pid       int              `json:"pid"`
	ExitCode  int              `json:"exit_code"`
	CreatedAt time.Time        `json:"created_at"`
	Bundle    string           `json:"bundle"`
	LogFile   string           `json:"log_file"`
	Readonly  bool             `json:"readonly"`
	Detached  bool             `json:"detached"`
	Env       []string         `json:"env,omitempty"`
	Cwd       string           `json:"cwd,omitempty"`
	Posture   security.Posture `json:"posture"`

	// NetnsPath joins an existing network namespace instead of creating an
	// empty one. The network package creates it before the container starts.
	NetnsPath string `json:"netns_path,omitempty"`
	// Publish records host-to-container port mappings so `stop` and `rm` can
	// remove their firewall rules.
	Publish []string `json:"publish,omitempty"`
}

// Seccomp is intentionally absent from the spec builder below.
// TODO(workstream 5): install a default seccomp deny-by-allowlist profile
// on spec.Linux.Seccomp and add MaskedPaths/ReadonlyPaths hardening.

// ---------------------------------------------------------------------------
// Spec building
// ---------------------------------------------------------------------------

// BuildSpec produces the OCI runtime-spec config.json contents for a
// container: namespaces (namespace.go), cgroups v2 limits (cgroups.go),
// the security posture (security package), and the standard /proc /dev /sys
// mounts runc requires us to declare explicitly.
func BuildSpec(c *Container, limits CgroupLimits) (*specs.Spec, error) {
	posture := c.Posture
	if posture.Seccomp == "" {
		posture = security.Default()
	}
	posture.Rootless = posture.Rootless && os.Getuid() != 0

	rootfsAbs, err := filepath.Abs(c.Rootfs)
	if err != nil {
		return nil, fmt.Errorf("resolve rootfs path: %w", err)
	}

	host := c.ID
	if len(host) > 12 {
		host = host[:12]
	}

	env := c.Env
	if len(env) == 0 {
		env = []string{
			"PATH=/usr/local/sbin:/usr/local/bin:/usr/sbin:/usr/bin:/sbin:/bin",
			"TERM=xterm",
			"HOME=/root",
		}
	}
	cwd := c.Cwd
	if cwd == "" {
		cwd = "/"
	}

	spec := &specs.Spec{
		Version: "1.1.0",
		Process: &specs.Process{
			Terminal: false,
			User:     specs.User{},
			Args:     c.Cmd,
			Env:      env,
			Cwd:      cwd,
		},
		Hostname: host,
		Root: &specs.Root{
			Path:     rootfsAbs,
			Readonly: posture.Readonly,
		},
		Linux: &specs.Linux{
			Namespaces: DefaultNamespaces(posture.Rootless, c.NetnsPath),
			Resources:  limits.Resources(),
		},
	}
	spec.Mounts = DefaultMounts(posture.Rootless)
	if posture.Rootless {
		uidMap, gidMap, err := security.Mappings()
		if err != nil {
			return nil, err
		}
		security.ApplyMappings(spec, uidMap, gidMap)
	}
	if err := security.Apply(spec, posture); err != nil {
		return nil, err
	}
	return spec, nil
}

// DefaultMounts declares the pseudo-filesystems every container needs.
// runc mounts nothing implicitly — an undeclared /proc means no /proc.
//
// The same set serves rootful and rootless runs: in the rootless case the
// container owns a fresh user, pid and network namespace, which is exactly
// what the kernel requires to permit a new procfs and sysfs instance.
// Binding the host's /proc instead is rejected by runc's proc-safety
// check, so it is deliberately not attempted.
func DefaultMounts(rootless bool) []specs.Mount {
	_ = rootless
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

	// The host's resolver configuration, so hostnames resolve. An image's
	// own /etc/resolv.conf names a resolver that does not exist on this
	// machine, and without this a container with working routing still
	// cannot reach anything by name.
	if _, err := os.Stat("/etc/resolv.conf"); err == nil {
		mounts = append(mounts, specs.Mount{
			Destination: "/etc/resolv.conf",
			Type:        "bind",
			Source:      "/etc/resolv.conf",
			Options:     []string{"rbind", "ro"},
		})
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

// StateRoot exposes the resolved state directory to sibling packages
// (image, overlayfs) that share the same content-addressable layout.
func StateRoot() (string, error) { return stateDir() }

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
