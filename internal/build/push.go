package build

import (
	"archive/tar"
	"compress/gzip"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strings"

	"github.com/google/go-containerregistry/pkg/v1"

	"sentra/internal/image"
)

// Packing a finished image for a registry.
//
// The build path never tars anything: layers are overlayfs upperdirs read
// in place. Push is different — a registry speaks OCI layer tarballs, so
// packing happens here, once, on the way out. Each build layer's tarball is
// content-addressed, so re-pushing an unchanged build reuses the file.

// PackLayers writes a gzipped OCI layer tarball for each build layer and
// returns their paths in order. baseRef and cfg describe the image being
// assembled around those layers.
func PackLayers(img *Image, stateRoot string) ([]string, error) {
	if len(img.BuildLayers) == 0 {
		return nil, fmt.Errorf("image %q has no build layers to push", img.Tag)
	}

	outDir := filepath.Join(stateRoot, "push-tmp")
	if err := os.MkdirAll(outDir, 0o755); err != nil {
		return nil, err
	}

	var paths []string
	for _, layer := range img.BuildLayers {
		if _, err := os.Stat(layer); err != nil {
			return nil, fmt.Errorf("build layer missing (rebuild, or the cache was pruned): %s", layer)
		}
		name, err := packLayer(layer, outDir)
		if err != nil {
			return nil, err
		}
		paths = append(paths, name)
	}
	return paths, nil
}

// packLayer tars a layer directory into <outDir>/<sha256>.tar.gz. The
// digest names the file, which makes repacking idempotent.
func packLayer(dir, outDir string) (string, error) {
	sum, err := dirDigest(dir)
	if err != nil {
		return "", err
	}
	dest := filepath.Join(outDir, sum+".tar.gz")
	if info, err := os.Stat(dest); err == nil && info.Size() > 0 {
		return dest, nil
	}

	f, err := os.Create(dest)
	if err != nil {
		return "", err
	}
	defer f.Close()

	gz := gzip.NewWriter(f)
	tw := tar.NewWriter(gz)

	walkErr := filepath.WalkDir(dir, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(dir, p)
		if err != nil {
			return err
		}
		if rel == "." {
			return nil
		}
		// OCI layer paths are always slash-separated and relative.
		name := path.Clean(filepath.ToSlash(rel))
		info, err := d.Info()
		if err != nil {
			return err
		}

		hdr := &tar.Header{
			Name:     name,
			Mode:     int64(info.Mode().Perm()),
			ModTime:  info.ModTime(),
			Typeflag: tar.TypeReg,
			Size:     info.Size(),
		}
		switch {
		case d.IsDir():
			hdr.Typeflag = tar.TypeDir
			hdr.Name += "/"
			hdr.Size = 0
		case d.Type()&os.ModeSymlink != 0:
			link, err := os.Readlink(p)
			if err != nil {
				return err
			}
			hdr.Typeflag = tar.TypeSymlink
			hdr.Linkname = link
			hdr.Size = 0
		case !info.Mode().IsRegular():
			// Device nodes and sockets have no place in a build diff.
			return nil
		}

		if err := tw.WriteHeader(hdr); err != nil {
			return err
		}
		if hdr.Typeflag != tar.TypeReg || hdr.Size == 0 {
			return nil
		}
		in, err := os.Open(p)
		if err != nil {
			return err
		}
		defer in.Close()
		_, err = io.Copy(tw, in)
		return err
	})
	if err := walkErr; err != nil {
		tw.Close()
		gz.Close()
		os.Remove(dest)
		return "", err
	}
	if err := tw.Close(); err != nil {
		gz.Close()
		os.Remove(dest)
		return "", err
	}
	if err := gz.Close(); err != nil {
		os.Remove(dest)
		return "", err
	}
	return dest, nil
}

// dirDigest hashes a layer's paths, modes and contents in a stable order,
// so the same layer always packs to the same tarball name.
func dirDigest(dir string) (string, error) {
	var files []string
	err := filepath.WalkDir(dir, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if p == dir {
			return nil
		}
		files = append(files, p)
		return nil
	})
	if err != nil {
		return "", err
	}
	sort.Strings(files)

	h := sha256.New()
	for _, p := range files {
		rel, _ := filepath.Rel(dir, p)
		info, err := os.Lstat(p)
		if err != nil {
			return "", err
		}
		fmt.Fprintf(h, "%s\x00%v\x00", filepath.ToSlash(rel), info.Mode().Perm())

		switch {
		case info.Mode().IsRegular():
			f, err := os.Open(p)
			if err != nil {
				return "", err
			}
			_, err = io.Copy(h, f)
			f.Close()
			if err != nil {
				return "", err
			}
		case info.Mode()&os.ModeSymlink != 0:
			link, err := os.Readlink(p)
			if err != nil {
				return "", err
			}
			io.WriteString(h, link)
		}
		h.Write([]byte{0})
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}

// PushArtifact uploads a built image to a registry and returns the stored
// manifest digest.
func PushArtifact(tag, destRef string) (*image.PushResult, error) {
	img, err := Load(tag)
	if err != nil {
		return nil, err
	}
	if img.Fallback {
		return nil, fmt.Errorf(
			"image %q was built without overlayfs layer support, so it has no per-step layers to push; rebuild as root (sudo sentra build -t %s .) for a pushable image",
			tag, tag)
	}
	stateRoot, err := StateRoot()
	if err != nil {
		return nil, err
	}
	paths, err := PackLayers(img, stateRoot)
	if err != nil {
		return nil, err
	}

	if destRef == "" {
		destRef = img.Tag
	}

	return image.Push(destRef, img.Base, paths, ImageConfig(img))
}

// ImageConfig translates a built image record into the OCI config file a
// registry expects.
func ImageConfig(img *Image) *v1.ConfigFile {
	cfg := &v1.ConfigFile{
		Config: v1.Config{
			Env:        append([]string(nil), img.Env...),
			Entrypoint: append([]string(nil), img.Entrypoint...),
		},
	}
	if len(img.Expose) > 0 {
		cfg.Config.ExposedPorts = map[string]struct{}{}
		for _, p := range img.Expose {
			if !strings.Contains(p, "/") {
				p += "/tcp"
			}
			cfg.Config.ExposedPorts[p] = struct{}{}
		}
	}
	return cfg
}
