package image

import (
	"archive/tar"
	"compress/gzip"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/google/go-containerregistry/pkg/v1"
)

// Layer fetching + extraction.
//
// Layout (under the Sentra state root):
//
//	blobs/sha256/<hex>   raw compressed layer blob, content-addressed
//	layers/sha256/<hex>/ extracted layer tree — one overlayfs lowerdir
//
// A blob is only downloaded if its extracted dir is missing; the download
// streams straight to disk while hashing, and is verified against the
// manifest digest before extraction.

// FetchLayer downloads layer into the CAS unless already present,
// verifying its digest. Returns the local blob path.
func FetchLayer(layer v1.Layer, blobsDir string) (string, error) {
	digest, err := layer.Digest()
	if err != nil {
		return "", fmt.Errorf("layer digest: %w", err)
	}
	blobPath := filepath.Join(blobsDir, digest.Algorithm, digest.Hex)
	if _, err := os.Stat(blobPath); err == nil {
		return blobPath, nil // cached from a previous pull
	}

	rc, err := layer.Compressed()
	if err != nil {
		return "", fmt.Errorf("open layer stream: %w", err)
	}
	defer rc.Close()

	tmp := blobPath + ".part"
	if err := os.MkdirAll(filepath.Dir(tmp), 0o755); err != nil {
		return "", err
	}
	f, err := os.Create(tmp)
	if err != nil {
		return "", err
	}
	h := sha256.New()
	_, copyErr := io.Copy(io.MultiWriter(f, h), rc)
	closeErr := f.Close()
	if copyErr != nil {
		os.Remove(tmp)
		return "", fmt.Errorf("download layer: %w", copyErr)
	}
	if closeErr != nil {
		os.Remove(tmp)
		return "", closeErr
	}

	got := hex.EncodeToString(h.Sum(nil))
	if got != digest.Hex {
		os.Remove(tmp)
		return "", fmt.Errorf("layer digest mismatch: got %s want %s", got, digest.Hex)
	}
	if err := os.Rename(tmp, blobPath); err != nil {
		return "", err
	}
	return blobPath, nil
}

// Extract unpacks a compressed OCI layer blob into dest. Handles Docker's
// whiteout convention (char device 0:0) by converting entries into the
// overlayfs ".wh." marker files that Merge() understands later.
func Extract(blobPath, dest string) error {
	if err := os.MkdirAll(dest, 0o755); err != nil {
		return err
	}
	f, err := os.Open(blobPath)
	if err != nil {
		return err
	}
	defer f.Close()

	gz, err := gzip.NewReader(f)
	if err != nil {
		return fmt.Errorf("gzip: %w", err) // non-gzip media types unsupported in MVP
	}
	defer gz.Close()

	tr := tar.NewReader(gz)
	for {
		hdr, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return fmt.Errorf("tar: %w", err)
		}
		if err := extractEntry(hdr, tr, dest); err != nil {
			return err
		}
	}

	// Completion marker — a sibling file, never a file inside the layer.
	// A marker inside the directory would become part of the image the
	// moment the directory is used as an overlayfs lowerdir.
	return os.WriteFile(dest+".complete", nil, 0o644)
}

func extractEntry(hdr *tar.Header, r io.Reader, dest string) error {
	// Anchor+Clean neutralizes path traversal ("../../etc/passwd") attempts.
	target := filepath.Join(dest, filepath.Clean("/"+hdr.Name))

	switch hdr.Typeflag {
	case tar.TypeDir:
		return mkdirAllMode(target, hdr.FileInfo().Mode())

	case tar.TypeReg:
		mode := hdr.FileInfo().Mode()
		out, err := os.OpenFile(target, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, mode.Perm())
		if err != nil {
			return err
		}
		if _, err := io.Copy(out, r); err != nil {
			out.Close()
			return err
		}
		if err := out.Close(); err != nil {
			return err
		}
		return os.Chtimes(target, hdr.AccessTime, hdr.ModTime)

	case tar.TypeSymlink:
		os.Remove(target)
		return os.Symlink(hdr.Linkname, target)

	case tar.TypeLink:
		os.Remove(target)
		src := filepath.Join(dest, filepath.Clean("/"+hdr.Linkname))
		return os.Link(src, target)

	case tar.TypeChar:
		// Docker/OCI whiteout convention: char device 0:0 named <x> means
		// "delete <x>" from lower layers. Record it as an overlayfs .wh.
		// marker so Merge() applies it during stacking.
		if hdr.Devmajor == 0 && hdr.Devminor == 0 {
			marker := filepath.Join(filepath.Dir(target), ".wh."+filepath.Base(target))
			return os.WriteFile(marker, nil, 0o644)
		}
		return nil // real device nodes skipped: never needed for base images

	default: // block/fifo/etc: not used by base images
		return nil
	}
}

func mkdirAllMode(path string, mode os.FileMode) error {
	if err := os.MkdirAll(path, mode.Perm()); err != nil {
		return err
	}
	return os.Chmod(path, mode.Perm())
}
