//go:build linux

package build

import (
	"os"

	"golang.org/x/sys/unix"
)

// copyFileRange moves file bytes with copy_file_range(2): the kernel
// copies between the two file descriptors without staging the data in
// user space, so large files never enter this process's memory.
//
// It is best-effort. Cross-filesystem pairs and filesystems without
// support return an error, and the caller falls back to a buffered copy.
func copyFileRange(src, dst string, mode os.FileMode) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()

	out, err := os.OpenFile(dst, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, mode)
	if err != nil {
		return err
	}
	defer out.Close()

	// The off_* arguments must be nil when passing an ordinary *os.File.
	var (
		remaining = int64(1) << 62
		copied    int64
	)
	for remaining > 0 {
		n, err := unix.CopyFileRange(int(in.Fd()), nil, int(out.Fd()), nil, int(remaining), 0)
		if n > 0 {
			copied += int64(n)
			remaining -= int64(n)
			continue
		}
		if err == unix.EINTR {
			continue
		}
		if err != nil {
			return err
		}
		break // short read with no error: done
	}

	st, err := out.Stat()
	if err == nil && st.Size() == copied {
		return out.Sync()
	}
	if err == nil && st.Size() != copied {
		return errShortCopy
	}
	return err
}
