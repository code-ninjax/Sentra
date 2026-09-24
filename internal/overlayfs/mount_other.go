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
