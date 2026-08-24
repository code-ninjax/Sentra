//go:build linux

package overlayfs

import (
	"fmt"
	"strings"
	"syscall"
)

// mountOverlay performs the real overlayfs kernel mount. Requires root
// (or CAP_SYS_ADMIN). Called from Merge; on failure Merge falls back to
// the userspace copy-merge.
func mountOverlay(lowerdirs []string, upper, work, merged string) error {
	opts := fmt.Sprintf("lowerdir=%s,upperdir=%s,workdir=%s",
		strings.Join(lowerdirs, ":"), upper, work)
	return syscall.Mount("overlay", merged, "overlay", 0, opts)
}
