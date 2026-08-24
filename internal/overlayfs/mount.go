package overlayfs

import (
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
)

// Merge stacks lowerdirs (bottom → top, i.e. base image layer first) into
// a single rootfs directory at <stateRoot>/rootfs/<name>.
//
// Two paths:
//  1. Real overlayfs kernel mount — used when running as root on Linux
//     (fast, zero-copy).
//  2. Userspace copy-merge fallback — used when not root or not Linux.
//     Slower (one file copy per changed file) but produces the identical
//     merged tree, and is what rootless Sentra uses until workstream 5
//     lands fuse-overlayfs support decisions.
func Merge(stateRoot, name string, lowerdirs []string) (string, error) {
	if len(lowerdirs) == 0 {
		return "", errors.New("no layers to merge")
	}
	base := filepath.Join(stateRoot, "rootfs")
	merged := filepath.Join(base, name)
	upper := filepath.Join(base, "."+name+"-upper")
	work := filepath.Join(base, "."+name+"-work")

	for _, dir := range []string{merged, upper, work} {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return "", fmt.Errorf("prepare %s: %w", dir, err)
		}
	}

	if err := mountOverlay(lowerdirs, upper, work, merged); err == nil {
		return merged, nil
	}
	return merged, copyMerge(lowerdirs, merged)
}

// copyMerge applies each layer in order onto merged, honoring .wh.
// whiteout markers and opaque-dir markers the same way overlayfs would.
// Later layers win; earlier files are overwritten in place.
func copyMerge(lowerdirs []string, merged string) error {
	for _, layer := range lowerdirs {
		if err := applyLayer(layer, merged); err != nil {
			return err
		}
	}
	return nil
}

func applyLayer(layer, dst string) error {
	return filepath.Walk(layer, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(layer, path)
		if err != nil {
			return err
		}
		if rel == "." {
			return nil // skip the layer root itself
		}
		name := info.Name()
		target := filepath.Join(dst, rel)

		// Whiteouts: ".wh.<victim>" deletes <victim> from the merge view;
		// ".wh..wh..opq" makes the containing directory opaque (clear it).
		if strings.HasPrefix(name, ".wh.") {
			victim := strings.TrimPrefix(name, ".wh.")
			if victim == ".wh..opq" {
				if err := clearDir(filepath.Dir(target)); err != nil {
					return err
				}
				return filepath.SkipDir
			}
			return os.RemoveAll(filepath.Join(filepath.Dir(target), victim))
		}
		// Extraction bookkeeping marker, never materialized into the merge.
		if name == ".complete" && !info.IsDir() {
			return nil
		}

		switch {
		case info.IsDir():
			return os.MkdirAll(target, info.Mode().Perm())
		case info.Mode()&os.ModeSymlink != 0:
			link, err := os.Readlink(path)
			if err != nil {
				return err
			}
			os.Remove(target)
			return os.Symlink(link, target)
		case info.Mode().IsRegular():
			return copyFile(path, target, info.Mode())
		default:
			return nil // devices/fifos skipped: base images don't need them
		}
	})
}

func clearDir(dir string) error {
	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}
	for _, e := range entries {
		if err := os.RemoveAll(filepath.Join(dir, e.Name())); err != nil {
			return err
		}
	}
	return nil
}

func copyFile(src, dst string, mode os.FileMode) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()

	out, err := os.OpenFile(dst, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, mode.Perm())
	if err != nil {
		return err
	}
	if _, err := io.Copy(out, in); err != nil {
		out.Close()
		return err
	}
	return out.Close()
}
