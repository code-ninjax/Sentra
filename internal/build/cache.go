package build

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"

	"sentra/internal/config"
)

// Layer caching. A step's cache key is the identity of its *inputs*:
//
//	sha256(step type, canonical args, parent layer id, input file hashes)
//
// so a build only re-runs a step when something that step actually read
// has changed. A Sentrafile `cache key=<value>` directive replaces the
// computed key outright, which is the manual invalidation escape hatch.
//
// Stored layout, under the Sentra state root:
//
//	build-cache/<key>/layer/   the step's overlayfs upperdir (its diff)
//	build-cache/<key>/meta.json
//
// Storing the diff rather than a full rootfs is what keeps builds cheap:
// cached steps cost one directory listing, never a filesystem copy.

// Cache stores per-step layer diffs, keyed by input identity.
type Cache struct {
	dir     string
	enabled bool

	mu      sync.Mutex
	hashing map[string]string // path+size+mtime -> digest (repeat-build fast path)
	hashMu  sync.Mutex
}

// NewCache prepares the cache directory. enabled=false makes every lookup
// miss, which is what --no-cache wants.
func NewCache(stateRoot string, enabled bool) (*Cache, error) {
	dir := filepath.Join(stateRoot, "build-cache")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, err
	}
	c := &Cache{dir: dir, enabled: enabled, hashing: map[string]string{}}
	if data, err := os.ReadFile(filepath.Join(dir, "hashindex.json")); err == nil {
		var index map[string]string
		if json.Unmarshal(data, &index) == nil {
			c.hashing = index
		}
	}
	return c, nil
}

// Entry is a cache hit: the recorded diff for a step.
type Entry struct {
	Key   string
	Layer string
}

// Lookup returns the cached layer for key, or ok=false on a miss.
func (c *Cache) Lookup(key string) (Entry, bool) {
	if !c.enabled {
		return Entry{}, false
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	layer := filepath.Join(c.dir, key, "layer")
	if info, err := os.Stat(layer); err != nil || !info.IsDir() {
		return Entry{}, false
	}
	return Entry{Key: key, Layer: layer}, true
}

// Store records a step's diff directory as the cached layer for key.
// The layer is moved into the cache, so the caller must not reuse it.
func (c *Cache) Store(key, layerDir string) (Entry, error) {
	if !c.enabled {
		return Entry{}, fmt.Errorf("cache disabled")
	}
	entry := filepath.Join(c.dir, key)
	if err := os.MkdirAll(entry, 0o755); err != nil {
		return Entry{}, err
	}
	dest := filepath.Join(entry, "layer")
	if err := os.RemoveAll(dest); err != nil {
		return Entry{}, err
	}
	if err := os.Rename(layerDir, dest); err != nil {
		// A cross-device rename (cache on another filesystem) degrades to
		// a copy; correctness first.
		if err := copyTree(layerDir, dest); err != nil {
			return Entry{}, err
		}
	}
	meta := map[string]any{"key": key, "stored": true}
	if data, err := json.Marshal(meta); err == nil {
		_ = os.WriteFile(filepath.Join(entry, "meta.json"), data, 0o644)
	}
	return Entry{Key: key, Layer: dest}, nil
}

// The v2 tag is the cache-key namespace version. Bump it whenever the
// meaning of a cached layer changes — a v1 entry produced by the older
// copy semantics must never be replayed into a v2 build.
const cacheKeyVersion = "sentra/auto/v2"
const manualKeyVersion = "sentra/manual/v2"

// ManualKey returns the cache key for a step carrying an explicit
// `cache key=<value>` override. The override deliberately bypasses input
// hashing and the parent layer: it is a promise by the Sentrafile author
// that this step's output can be reused under this identity.
func ManualKey(override string) string {
	sum := sha256.Sum256([]byte(manualKeyVersion + "\x00" + override))
	return hex.EncodeToString(sum[:])
}

// AutoKey computes a step's cache key from its identity and the content
// of the files it reads. parent is the id of the layer stack beneath it;
// inputs is one digest per source file, already hashed by HashInputs.
func AutoKey(step config.Step, parent string, inputs []InputDigest) string {
	h := sha256.New()
	fmt.Fprintf(h, "%s\x00%s\x00%s\x00", cacheKeyVersion, step.Type, parent)

	switch step.Type {
	case config.DirectiveCopy:
		fmt.Fprintf(h, "from=%s\x00src=%s\x00dest=%s\x00", step.From, step.Src, step.Dest)
		for _, in := range inputs {
			fmt.Fprintf(h, "in=%s:%s\x00", in.Path, in.Digest)
		}
	case config.DirectiveExec:
		fmt.Fprintf(h, "argv=%s\x00", strings.Join(step.Argv, "\x01"))
	case config.DirectiveWorkdir:
		fmt.Fprintf(h, "path=%s\x00", step.Path)
	case config.DirectiveEnv:
		fmt.Fprintf(h, "env=%s\x00", strings.Join(step.Env, "\x01"))
	case config.DirectiveExpose:
		fmt.Fprintf(h, "ports=%s\x00", strings.Join(step.Ports, "\x01"))
	case config.DirectiveStart:
		fmt.Fprintf(h, "argv=%s\x00", strings.Join(step.Argv, "\x01"))
	}
	return hex.EncodeToString(h.Sum(nil))
}

// InputDigest is one hashed build-context input.
type InputDigest struct {
	Path   string // relative to the context root
	Digest string
}

// HashInputs hashes the files a copy step reads, relative to context.
// Directory sources are walked recursively. Results are sorted by path so
// the key is independent of filesystem walk order.
//
// Hashing runs on a bounded worker pool: it is CPU-bound and a weak CPU
// with four threads should be used fully without spawning a goroutine per
// file.
func HashInputs(c *Cache, context, src string, workers int) ([]InputDigest, error) {
	full := filepath.Join(context, filepath.FromSlash(src))
	info, err := os.Stat(full)
	if err != nil {
		return nil, fmt.Errorf("copy source %q: %w", src, err)
	}

	var paths []string
	if info.IsDir() {
		err = filepath.WalkDir(full, func(p string, d fs.DirEntry, err error) error {
			if err != nil {
				return err
			}
			if d.IsDir() {
				return nil
			}
			if d.Type()&fs.ModeSymlink != 0 {
				return nil // symlinks carry no content; copying re-creates them
			}
			paths = append(paths, p)
			return nil
		})
		if err != nil {
			return nil, err
		}
	} else {
		paths = []string{full}
	}
	sort.Strings(paths)

	if workers < 1 {
		workers = 1
	}
	out := make([]InputDigest, len(paths))
	sem := make(chan struct{}, workers)
	var wg sync.WaitGroup
	var firstErr error
	var errMu sync.Mutex

	for i, p := range paths {
		wg.Add(1)
		go func(i int, p string) {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()

			rel, err := filepath.Rel(context, p)
			if err != nil {
				rel = p
			}
			digest, err := c.hashFile(p)
			if err != nil {
				errMu.Lock()
				if firstErr == nil {
					firstErr = err
				}
				errMu.Unlock()
				return
			}
			out[i] = InputDigest{Path: filepath.ToSlash(rel), Digest: digest}
		}(i, p)
	}
	wg.Wait()
	if firstErr != nil {
		return nil, firstErr
	}
	return out, nil
}

// hashFile streams a file through sha256 with a fixed 64 KiB buffer, so
// memory use stays flat regardless of file size. Repeat builds reuse the
// digest when size and mtime are unchanged, which is the difference
// between an instant and a slow rebuild on modest hardware.
func (c *Cache) hashFile(path string) (string, error) {
	info, err := os.Stat(path)
	if err != nil {
		return "", err
	}
	key := hashIndexKey(path, info)

	c.hashMu.Lock()
	if cached, ok := c.hashing[key]; ok {
		c.hashMu.Unlock()
		return cached, nil
	}
	c.hashMu.Unlock()

	f, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer f.Close()

	h := sha256.New()
	buf := make([]byte, 64*1024)
	if _, err := io.CopyBuffer(h, f, buf); err != nil {
		return "", err
	}
	digest := hex.EncodeToString(h.Sum(nil))

	c.hashMu.Lock()
	c.hashing[key] = digest
	c.hashMu.Unlock()
	return digest, nil
}

func hashIndexKey(path string, info os.FileInfo) string {
	return fmt.Sprintf("%s|%d|%d", filepath.ToSlash(path), info.Size(), info.ModTime().UnixNano())
}

// SaveHashIndex persists the path→digest index for the next build.
func (c *Cache) SaveHashIndex() error {
	c.hashMu.Lock()
	defer c.hashMu.Unlock()
	data, err := json.Marshal(c.hashing)
	if err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(c.dir, "hashindex.json"), data, 0o644)
}

func copyTree(src, dst string) error {
	return filepath.WalkDir(src, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(src, p)
		if err != nil {
			return err
		}
		target := filepath.Join(dst, rel)
		if d.IsDir() {
			return os.MkdirAll(target, 0o755)
		}
		if !d.Type().IsRegular() {
			return nil
		}
		return copyRegular(p, target)
	})
}
