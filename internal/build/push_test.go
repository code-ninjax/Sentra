package build

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/google/go-containerregistry/pkg/name"
	"github.com/google/go-containerregistry/pkg/v1/tarball"

	"sentra/internal/image"
)

// The push path is the one place Sentra produces a brand-new OCI image, so
// it gets checked without a registry: the assembled image is written to a
// docker-archive tarball and read back. A round trip that survives that is
// the same artifact remote.Write would upload, proven valid.

func TestPackLayerRoundTrip(t *testing.T) {
	layerDir := t.TempDir()
	writeFile(t, filepath.Join(layerDir, "app", "binary"), "#!/bin/sh\necho built\n")
	writeFile(t, filepath.Join(layerDir, "app", "lib", "data.txt"), "payload")
	if err := os.MkdirAll(filepath.Join(layerDir, "app", "link"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink("../binary", filepath.Join(layerDir, "app", "link", "to-binary")); err != nil {
		t.Skipf("symlinks unavailable: %v", err)
	}

	// The packed layer is written outside t.TempDir() on purpose:
	// go-containerregistry holds the file open, and Windows refuses to
	// remove a file with an open handle.
	outDir, err := os.MkdirTemp("", "sentra-pack")
	if err != nil {
		t.Fatalf("temp dir: %v", err)
	}
	defer func() { _ = os.RemoveAll(outDir) }()

	path, err := packLayer(layerDir, outDir)
	if err != nil {
		t.Fatalf("packLayer: %v", err)
	}
	if !strings.HasSuffix(path, ".tar.gz") {
		t.Errorf("packed layer = %q, want a .tar.gz", path)
	}
	if info, err := os.Stat(path); err != nil || info.Size() == 0 {
		t.Fatalf("packed layer is empty: %v", err)
	}

	// Packing is content-addressed, so the same input yields the same file.
	again, err := packLayer(layerDir, outDir)
	if err != nil {
		t.Fatalf("packLayer rerun: %v", err)
	}
	if again != path {
		t.Errorf("repack produced %q, want the same content-addressed %q", again, path)
	}

	// go-containerregistry must accept it as a real OCI layer.
	layer, err := tarball.LayerFromFile(path)
	if err != nil {
		t.Fatalf("LayerFromFile: %v", err)
	}
	digest, err := layer.Digest()
	if err != nil {
		t.Fatalf("layer digest: %v", err)
	}
	if digest.Hex == "" {
		t.Error("layer has no digest")
	}
	if _, err := layer.Compressed(); err != nil {
		t.Errorf("layer compressed stream: %v", err)
	}
}

func TestImageConfigFromRecord(t *testing.T) {
	img := &Image{
		Env:        []string{"PORT=3000"},
		Expose:     []string{"8080", "5353/udp"},
		Entrypoint: []string{"npm", "start"},
	}
	cfg := ImageConfig(img)

	if got := strings.Join(cfg.Config.Entrypoint, " "); got != "npm start" {
		t.Errorf("entrypoint = %q", got)
	}
	if got := strings.Join(cfg.Config.Env, ","); got != "PORT=3000" {
		t.Errorf("env = %q", got)
	}
	if _, ok := cfg.Config.ExposedPorts["8080/tcp"]; !ok {
		t.Error("8080 was not normalized to 8080/tcp")
	}
	if _, ok := cfg.Config.ExposedPorts["5353/udp"]; !ok {
		t.Error("5353/udp was not preserved")
	}
}

// TestAssembleProducesReadableImage builds the exact artifact push would
// upload and round-trips it through a docker-archive. It needs the registry
// to fetch the base, so it is opt-in.
func TestAssembleProducesReadableImage(t *testing.T) {
	if os.Getenv("SENTRA_TEST_NETWORK") == "" {
		t.Skip("set SENTRA_TEST_NETWORK=1 to exercise the registry-backed path")
	}

	layerDir := t.TempDir()
	writeFile(t, filepath.Join(layerDir, "app", "marker"), "from sentra build")
	packedDir, err := os.MkdirTemp("", "sentra-pack")
	if err != nil {
		t.Fatalf("temp dir: %v", err)
	}
	defer func() { _ = os.RemoveAll(packedDir) }()
	packed, err := packLayer(layerDir, packedDir)
	if err != nil {
		t.Fatalf("packLayer: %v", err)
	}

	img, err := image.Assemble("alpine:3.20", []string{packed},
		ImageConfig(&Image{Entrypoint: []string{"/bin/sh"}}))
	if err != nil {
		t.Fatalf("Assemble: %v", err)
	}

	manifest, err := img.Manifest()
	if err != nil {
		t.Fatalf("manifest: %v", err)
	}
	if len(manifest.Layers) == 0 {
		t.Fatal("assembled image has no layers")
	}

	// The config must describe a runnable rootfs: one diff ID per layer.
	// Replacing the config instead of merging it silently drops these, and
	// the image then fails to start anywhere.
	cfg, err := img.ConfigFile()
	if err != nil {
		t.Fatalf("config: %v", err)
	}
	if got := len(cfg.RootFS.DiffIDs); got != len(manifest.Layers) {
		t.Errorf("rootfs.diff_ids = %d, want one per layer (%d)", got, len(manifest.Layers))
	}
	if got := strings.Join(cfg.Config.Entrypoint, " "); got != "/bin/sh" {
		t.Errorf("entrypoint = %q, want /bin/sh (the override must survive)", got)
	}

	ref, err := name.ParseReference("localhost:5000/sentra-test:latest")
	if err != nil {
		t.Fatalf("parse reference: %v", err)
	}
	archive := filepath.Join(t.TempDir(), "image.tar")
	if err := tarball.WriteToFile(archive, ref, img); err != nil {
		t.Fatalf("WriteToFile: %v", err)
	}

	// If another OCI implementation can load what we built, remote.Write can
	// ship it.
	loaded, err := tarball.ImageFromPath(archive, nil)
	if err != nil {
		t.Fatalf("ImageFromPath: %v", err)
	}
	loadedManifest, err := loaded.Manifest()
	if err != nil {
		t.Fatalf("loaded manifest: %v", err)
	}
	if len(loadedManifest.Layers) != len(manifest.Layers) {
		t.Errorf("round trip changed layer count: %d -> %d",
			len(manifest.Layers), len(loadedManifest.Layers))
	}
}
