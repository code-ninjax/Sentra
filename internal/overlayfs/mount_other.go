//go:build !linux

package overlayfs

import "errors"

// mountOverlay is a no-op off Linux: Merge() falls back to the userspace
// copy-merge, which produces an identical tree.
func mountOverlay(lowerdirs []string, upper, work, merged string) error {
	_ = lowerdirs
	_ = upper
	_ = work
	_ = merged
	return errors.New("overlayfs mounts are Linux-only")
}

// MountStep is unavailable off Linux; the build engine falls back to a
// single writable rootfs copy.
func MountStep(lowerdirs []string, upper, work, mountpoint string) (func() error, error) {
	return nil, errors.New("overlayfs mounts are Linux-only")
}

// Probe reports that layering is unavailable off Linux.
func Probe(lower, upper, work, mountpoint string) error {
	return errors.New("overlayfs mounts are Linux-only")
}
