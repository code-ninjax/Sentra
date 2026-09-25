package build

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	rt "sentra/internal/runtime"
)

// StateRoot returns the resolved Sentra state directory.
func StateRoot() (string, error) { return rt.StateRoot() }

// Load reads the record a previous `sentra build -t TAG` wrote, so other
// commands can consume a built image by tag instead of by filesystem path.
//
// This is what makes `sentra run -t myapp` work: the build already knows
// the assembled rootfs, the environment and the entrypoint, and the
// filesystem layout stays an implementation detail.
func Load(tag string) (*Image, error) {
	stateRoot, err := StateRoot()
	if err != nil {
		return nil, err
	}
	return LoadFrom(stateRoot, tag)
}

// LoadFrom reads a built image record from an explicit state root.
func LoadFrom(stateRoot, tag string) (*Image, error) {
	name := sanitizeTag(tag)
	path := filepath.Join(stateRoot, "built", name+".json")

	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, fmt.Errorf("no built image tagged %q (run: sentra build -t %s .)", tag, tag)
		}
		return nil, err
	}

	img := &Image{}
	if err := json.Unmarshal(data, img); err != nil {
		return nil, fmt.Errorf("corrupt image record for %q: %w", tag, err)
	}
	if img.Rootfs == "" {
		return nil, fmt.Errorf("image record for %q has no rootfs", tag)
	}
	if _, err := os.Stat(img.Rootfs); err != nil {
		return nil, fmt.Errorf("rootfs for %q is missing (rebuild, or re-pull the base): %w", tag, err)
	}
	return img, nil
}

// List returns every built image tag, newest first.
func List() ([]string, error) {
	stateRoot, err := rt.StateRoot()
	if err != nil {
		return nil, err
	}
	entries, err := os.ReadDir(filepath.Join(stateRoot, "built"))
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}

	type record struct {
		tag string
		at  int64
	}
	var found []record
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".json") {
			continue
		}
		data, err := os.ReadFile(filepath.Join(stateRoot, "built", e.Name()))
		if err != nil {
			continue
		}
		img := &Image{}
		if json.Unmarshal(data, img) != nil {
			continue
		}
		found = append(found, record{
			tag: img.Tag,
			at:  img.BuiltAt.UnixNano(),
		})
	}
	// Newest first, without pulling in sort.SliceStable's reflection.
	for i := 1; i < len(found); i++ {
		for j := i; j > 0 && found[j].at > found[j-1].at; j-- {
			found[j], found[j-1] = found[j-1], found[j]
		}
	}

	tags := make([]string, 0, len(found))
	for _, r := range found {
		tags = append(tags, r.tag)
	}
	return tags, nil
}

// Remove deletes a built image's record. The rootfs and cached layers are
// left alone: workstream 7 owns storage retention.
func Remove(tag string) error {
	stateRoot, err := rt.StateRoot()
	if err != nil {
		return err
	}
	path := filepath.Join(stateRoot, "built", sanitizeTag(tag)+".json")
	if err := os.Remove(path); err != nil {
		if os.IsNotExist(err) {
			return fmt.Errorf("no built image tagged %q", tag)
		}
		return err
	}
	return nil
}
