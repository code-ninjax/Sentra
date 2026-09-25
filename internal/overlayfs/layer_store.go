package overlayfs

import (
	"os"
	"path/filepath"
)

// LayerStore: which layer digests are already extracted on disk.
//
// Keyed by sha256 hex under <stateRoot>/layers/<hex>/. This is also the
// foundation workstream 4's build cache builds on — same rule applies
// there: known digest = skip the work entirely.

func LayersDir(stateRoot string) string {
	return filepath.Join(stateRoot, "layers")
}

// LayerDir is where layer with digest hex should be extracted.
func LayerDir(stateRoot, hex string) string {
	return filepath.Join(LayersDir(stateRoot), hex)
}

// HasLayer reports whether a layer is fully extracted. The completion
// marker is a sibling file written only after Extract() finishes, so a
// half-extracted layer from a crashed pull never counts, and the marker
// itself never leaks into an image as a regular file.
func HasLayer(stateRoot, hex string) bool {
	_, err := os.Stat(LayerDir(stateRoot, hex) + ".complete")
	return err == nil
}
