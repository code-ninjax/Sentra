//go:build linux

package overlayfs

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
)

// mountOverlay performs the real overlayfs kernel mount. Requires root
// (or CAP_SYS_ADMIN). Called from Merge; on failure Merge falls back to
// the userspace copy-merge.
func mountOverlay(lowerdirs []string, upper, work, merged string) error {
	// Re-mounting on top of a live overlay that shares the same upperdir
	// silently corrupts the view: the kernel stacks a second mount over
	// the first, and lookups against the shared upper start failing. Any
	// mount we previously placed here must come off first.
	unmountIfMounted(merged)

	// Sentra represents a layer chain bottom-to-top so copyMerge can apply
	// it in order. Overlayfs gives the leftmost lowerdir highest precedence,
	// so reverse the chain only at the kernel-mount boundary.
	kernelLowers := make([]string, len(lowerdirs))
	for i, lower := range lowerdirs {
		kernelLowers[len(lowerdirs)-1-i] = lower
	}
	opts := fmt.Sprintf("lowerdir=%s,upperdir=%s,workdir=%s",
		strings.Join(kernelLowers, ":"), upper, work)
	return syscall.Mount("overlay", merged, "overlay", 0, opts)
}

// unmountIfMounted clears an existing mount at path, if there is one.
// Reading mountinfo rather than blindly unmounting keeps us from tearing
// down something that is not ours.
func unmountIfMounted(path string) {
	data, err := os.ReadFile("/proc/self/mountinfo")
	if err != nil {
		return
	}
	for _, line := range strings.Split(string(data), "\n") {
		fields := strings.Fields(line)
		if len(fields) < 5 {
			continue
		}
		if unescapeMountField(fields[4]) != path {
			continue
		}
		_ = syscall.Unmount(path, syscall.MNT_DETACH)
		return
	}
}

// unescapeMountField reverses the octal escaping the kernel applies to
// spaces and backslashes in mountinfo paths.
func unescapeMountField(s string) string {
	if !strings.Contains(s, `\`) {
		return s
	}
	var b strings.Builder
	for i := 0; i < len(s); i++ {
		if s[i] == '\\' && i+3 < len(s) {
			if v, err := strconv.ParseUint(s[i+1:i+4], 8, 8); err == nil {
				b.WriteByte(byte(v))
				i += 3
				continue
			}
		}
		b.WriteByte(s[i])
	}
	return b.String()
}

// MountStep mounts an overlay stack and returns a teardown function.
// The build engine uses this to give every step its own writable upperdir
// while reading the layers beneath it.
func MountStep(lowerdirs []string, upper, work, mountpoint string) (func() error, error) {
	// A previous build killed mid-step can leave the shared mountpoint
	// mounted, which would make this mount fail with EBUSY. Clear it first.
	_ = syscall.Unmount(mountpoint, syscall.MNT_DETACH)

	if err := mountOverlay(lowerdirs, upper, work, mountpoint); err != nil {
		return nil, err
	}
	return func() error {
		return syscall.Unmount(mountpoint, syscall.MNT_DETACH)
	}, nil
}

// Probe verifies that overlayfs mounts work in this environment, so the
// build engine can choose its layered path up front instead of failing
// halfway through a build.
func Probe(lower, upper, work, mountpoint string) error {
	const probeFile = ".sentra-overlay-probe"
	probePath := filepath.Join(lower, probeFile)
	if err := os.WriteFile(probePath, []byte("ok"), 0o644); err != nil {
		return fmt.Errorf("prepare overlayfs probe: %w", err)
	}
	defer os.Remove(probePath)

	unmount, err := MountStep([]string{lower}, upper, work, mountpoint)
	if err != nil {
		return fmt.Errorf("overlayfs mount: %w", err)
	}
	defer unmount()

	if _, err := os.Stat(filepath.Join(mountpoint, probeFile)); err != nil {
		return fmt.Errorf("overlayfs mount does not expose its lowerdir: %w", err)
	}
	return nil
}
