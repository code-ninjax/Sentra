package security

// Posture is the resolved runtime security policy for one container: how
// isolated it is, whether its rootfs can be written, and how tightly its
// syscalls are filtered.
//
// A Posture comes from three places, in increasing priority: Sentra's
// defaults, a Sentrafile's `secure` directive, and explicit `sentra run`
// flags. Keeping it as one value means the runtime never has to guess, and
// `sentra run -t myapp` automatically gets the posture the build declared.

import (
	"errors"

	"github.com/opencontainers/runtime-spec/specs-go"
)

var errNilSpec = errors.New("security: spec has no linux section to apply a posture to")

// Posture is the policy applied to a container.
type Posture struct {
	// Rootless runs the container in a user namespace owned by the
	// invoking user instead of requiring root.
	Rootless bool
	// Readonly mounts the rootfs read-only. A container that needs to
	// write declares writable paths instead of a writable root.
	Readonly bool
	// Seccomp selects the syscall filter.
	Seccomp Level
}

// Default returns Sentra's baseline: rootless when possible, read-only
// rootfs, and the broad seccomp allowlist. This mirrors the posture the
// Sentrafile parser assumes when no `secure` directive is present.
func Default() Posture {
	return Posture{
		Rootless: true,
		Readonly: true,
		Seccomp:  LevelDefault,
	}
}

// maskedPaths are bind-mounted over with /dev/null so a container cannot
// read host kernel state through them.
var maskedPaths = []string{
	"/proc/acpi",
	"/proc/kcore",
	"/proc/keys",
	"/proc/latency_stats",
	"/proc/timer_list",
	"/proc/timer_stats",
	"/proc/sched_debug",
	"/proc/scsi",
	"/sys/firmware",
	"/sys/devices/virtual/powercap",
}

// readonlyPaths are made read-only inside the mount namespace, so a
// container cannot reconfigure the kernel it shares.
var readonlyPaths = []string{
	"/proc/asound",
	"/proc/bus",
	"/proc/fs",
	"/proc/irq",
	"/proc/sys",
	"/proc/sysrq-trigger",
}

// Apply writes the posture into an OCI spec. It is the single place that
// turns policy into kernel-visible configuration.
//
// The spec must already carry a non-nil Linux section; callers building a
// spec from scratch should use runtime.BuildSpec.
func Apply(spec *specs.Spec, p Posture) error {
	if spec == nil || spec.Linux == nil {
		return errNilSpec
	}
	if spec.Root == nil {
		spec.Root = &specs.Root{Path: "/"}
	}

	spec.Root.Readonly = p.Readonly

	profile, err := Profile(p.Seccomp)
	if err != nil {
		return err
	}
	spec.Linux.Seccomp = profile

	// Path masking is applied only when Sentra is rootful. runc creates the
	// masked mountpoints and fchowns them to match their parent directory,
	// which a user namespace cannot do, so asking for it rootless fails the
	// whole container start. The protection is not lost: a rootless
	// container gets a fresh pid namespace, so /proc/kcore and friends
	// describe the container rather than the host, and the user namespace
	// already withholds the capabilities that make them readable.
	if p.Readonly && !p.Rootless {
		spec.Linux.MaskedPaths = append([]string(nil), maskedPaths...)
		spec.Linux.ReadonlyPaths = append([]string(nil), readonlyPaths...)
	} else {
		spec.Linux.MaskedPaths = nil
		spec.Linux.ReadonlyPaths = nil
	}
	return nil
}

// Summary renders a posture for `sentra run` reporting.
func (p Posture) Summary() string {
	mode := "rootful"
	if p.Rootless {
		mode = "rootless"
	}
	rootfs := "writable"
	if p.Readonly {
		rootfs = "read-only"
	}
	return mode + ", " + rootfs + " rootfs, seccomp=" + string(p.Seccomp)
}
