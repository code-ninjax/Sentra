package build

import (
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
)

// COPY step implementation.
//
// Large regular files are moved with copy_file_range(2) where the kernel
// supports it, which keeps the data out of user space entirely. Small
// files use an ordinary buffered copy: below the threshold the syscall
// round-trip costs more than the copy itself. Anything copy_file_range
// refuses (different filesystems, unsupported fs) falls back rather than
// failing the build.

// zeroCopyThreshold is the size at which copy_file_range starts paying
// for itself. Tuned low deliberately — correctness never depends on it,
// only speed.
const zeroCopyThreshold = 64 * 1024

var errShortCopy = errors.New("copy_file_range produced a short copy")

// copyRegular copies one regular file, preserving its permission bits.
func copyRegular(src, dst string) error {
	info, err := os.Stat(src)
	if err != nil {
		return err
	}
	mode := info.Mode().Perm()
	if mode == 0 {
		mode = 0o644
	}

	if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
		return err
	}

	if info.Size() >= zeroCopyThreshold {
		if err := copyFileRange(src, dst, mode); err == nil {
			return nil
		}
	}

	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()

	out, err := os.OpenFile(dst, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, mode)
	if err != nil {
		return err
	}
	buf := make([]byte, 64*1024)
	if _, err := io.CopyBuffer(out, in, buf); err != nil {
		out.Close()
		return err
	}
	return out.Close()
}

// copyInto places a build-context source at dest inside root, using
// Docker's familiar copy semantics: a directory source copies its
// contents into dest, and a dest that names a directory (or ends in "/")
// receives the source under its own name.
func copyInto(context, root, src, dst string) error {
	// Sources are resolved against the source root, which is the build
	// context normally and an earlier stage's rootfs for `copy --from=`.
	// An absolute source is anchored to that root rather than the host, and
	// traversal outside it is rejected.
	absContext, err := filepath.Abs(context)
	if err != nil {
		return err
	}
	slashSrc := strings.TrimPrefix(trimSlash(filepath.ToSlash(src)), "/")
	srcPath := filepath.Clean(filepath.Join(absContext, filepath.FromSlash(slashSrc)))
	if srcPath != absContext && !strings.HasPrefix(srcPath, absContext+string(os.PathSeparator)) {
		return fmt.Errorf("copy source %q escapes the build context", src)
	}

	info, err := os.Lstat(srcPath)
	if err != nil {
		return fmt.Errorf("copy source %q: %w", src, err)
	}

	dstPath, err := resolveDest(root, srcPath, dst, info.IsDir())
	if err != nil {
		return err
	}

	if info.IsDir() {
		return copyDir(srcPath, dstPath)
	}
	if info.Mode()&os.ModeSymlink != 0 {
		link, err := os.Readlink(srcPath)
		if err != nil {
			return err
		}
		if err := os.MkdirAll(filepath.Dir(dstPath), 0o755); err != nil {
			return err
		}
		os.Remove(dstPath)
		return os.Symlink(link, dstPath)
	}
	if !info.Mode().IsRegular() {
		return fmt.Errorf("copy source %q is not a regular file", src)
	}
	return copyRegular(srcPath, dstPath)
}

func copyDir(src, dst string) error {
	if err := os.MkdirAll(dst, 0o755); err != nil {
		return err
	}
	return filepath.WalkDir(src, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(src, p)
		if err != nil {
			return err
		}
		if rel == "." {
			return nil
		}
		target := filepath.Join(dst, rel)

		switch {
		case d.IsDir():
			return os.MkdirAll(target, 0o755)
		case d.Type()&os.ModeSymlink != 0:
			link, err := os.Readlink(p)
			if err != nil {
				return err
			}
			os.Remove(target)
			return os.Symlink(link, target)
		case d.Type().IsRegular():
			return copyRegular(p, target)
		default:
			return nil // devices and sockets never appear in build context
		}
	})
}

// resolveDest maps a Sentrafile destination onto a path inside the build
// root, rejecting anything that would escape it.
func resolveDest(root, srcPath, dst string, srcIsDir bool) (string, error) {
	clean := filepath.Clean(filepath.FromSlash(dst))
	if clean == "." {
		clean = ""
	}

	// Join handles both forms: an absolute dest is treated as relative to
	// the build root, never to the host filesystem.
	target := filepath.Join(root, clean)

	if target != root && !strings.HasPrefix(target, root+string(os.PathSeparator)) {
		return "", fmt.Errorf("copy destination %q escapes the build root", dst)
	}

	intoDir := strings.HasSuffix(dst, "/") || isDir(target)
	if srcIsDir {
		// A directory always copies its contents into dest.
		return target, nil
	}
	if intoDir {
		return filepath.Join(target, filepath.Base(srcPath)), nil
	}
	return target, nil
}

func isDir(path string) bool {
	info, err := os.Stat(path)
	return err == nil && info.IsDir()
}

func trimSlash(s string) string {
	for len(s) > 1 && s[len(s)-1] == '/' {
		s = s[:len(s)-1]
	}
	return s
}
