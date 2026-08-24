package image

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"sentra/internal/overlayfs"
	"sentra/internal/runtime"
)

// Pull implements `sentra pull <ref>`:
//
//	resolve ref → manifest → per-layer fetch (CAS, digest-verified) →
//	extract to layers/<digest>/ → merge into rootfs/<name>/
//
// Returns the merged rootfs path, ready for `sentra run --rootfs <path>`.

// imageRecord is the persisted metadata for a pulled ref. Kept minimal —
// enough to know which layer dirs compose an image without re-hitting the
// registry.
type imageRecord struct {
	Ref      string   `json:"ref"`
	Digest   string   `json:"digest"`
	LayerIDs []string `json:"layer_ids"` // sha256 hexes, bottom → top
}

func Pull(refStr string) (string, error) {
	stateRoot, err := runtime.StateRoot()
	if err != nil {
		return "", err
	}
	blobsDir := filepath.Join(stateRoot, "blobs")

	reg := NewRegistry()
	ref, img, err := reg.Image(refStr)
	if err != nil {
		return "", err
	}

	digest, err := img.Digest()
	if err != nil {
		return "", fmt.Errorf("image digest: %w", err)
	}
	fmt.Printf("pulling %s@%s\n", ref.Name(), digest)

	layers, err := img.Layers()
	if err != nil {
		return "", fmt.Errorf("manifest layers: %w", err)
	}

	var hexes []string
	var lowerdirs []string
	for i, layer := range layers {
		h, err := layer.Digest()
		if err != nil {
			return "", err
		}
		hexes = append(hexes, h.Hex)
		dir := overlayfs.LayerDir(stateRoot, h.Hex)

		if overlayfs.HasLayer(stateRoot, h.Hex) {
			fmt.Printf("  layer %d/%d cached (%s)\n", i+1, len(layers), short(h.Hex))
		} else {
			size, _ := layer.Size()
			fmt.Printf("  layer %d/%d fetching %s (%d bytes)...\n", i+1, len(layers), short(h.Hex), size)
			blobPath, err := FetchLayer(layer, blobsDir)
			if err != nil {
				return "", err
			}
			if err := Extract(blobPath, dir); err != nil {
				return "", fmt.Errorf("extract layer %d: %w", i+1, err)
			}
		}
		lowerdirs = append(lowerdirs, dir)
	}

	rec := imageRecord{Ref: ref.Name(), Digest: digest.String(), LayerIDs: hexes}
	if err := saveRecord(stateRoot, rec); err != nil {
		return "", err
	}

	name := sanitize(ref.Context().RepositoryStr() + "-" + ref.Identifier())
	rootfsPath, err := overlayfs.Merge(stateRoot, name, lowerdirs)
	if err != nil {
		return "", fmt.Errorf("merge rootfs: %w", err)
	}
	fmt.Printf("rootfs ready: %s\n", rootfsPath)
	return rootfsPath, nil
}

func saveRecord(stateRoot string, rec imageRecord) error {
	path := filepath.Join(stateRoot, "images.json")
	var records map[string]imageRecord
	if data, err := os.ReadFile(path); err == nil {
		if json.Unmarshal(data, &records) != nil {
			records = map[string]imageRecord{}
		}
	} else {
		records = map[string]imageRecord{}
	}
	records[rec.Ref] = rec
	data, err := json.MarshalIndent(records, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0o644)
}

func short(hex string) string {
	if len(hex) > 12 {
		return hex[:12]
	}
	return hex
}

func sanitize(s string) string {
	out := make([]rune, 0, len(s))
	for _, r := range s {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9', r == '-', r == '_', r == '.':
			out = append(out, r)
		default:
			out = append(out, '-')
		}
	}
	return string(out)
}
