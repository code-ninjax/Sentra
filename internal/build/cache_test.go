package build

import (
	"os"
	"path/filepath"
	"testing"

	"sentra/internal/config"
)

func TestAutoKeyIsContentAddressed(t *testing.T) {
	copyStep := config.Step{Type: config.DirectiveCopy, Src: "app", Dest: "."}
	inputs := []InputDigest{{Path: "app/main.js", Digest: "aaa"}, {Path: "app/util.js", Digest: "bbb"}}

	a := AutoKey(copyStep, "base:xyz", inputs)
	b := AutoKey(copyStep, "base:xyz", inputs)
	if a != b {
		t.Fatalf("same inputs produced different keys:\n%s\n%s", a, b)
	}

	changed := []InputDigest{{Path: "app/main.js", Digest: "CHANGED"}, {Path: "app/util.js", Digest: "bbb"}}
	if AutoKey(copyStep, "base:xyz", changed) == a {
		t.Error("changing an input digest did not change the key")
	}
	if AutoKey(copyStep, "base:DIFFERENT", inputs) == a {
		t.Error("changing the parent layer did not change the key")
	}
	if AutoKey(copyStep, "base:xyz", inputs[:1]) == a {
		t.Error("removing an input did not change the key")
	}
}

func TestAutoKeyDistinguishesDirectives(t *testing.T) {
	copyStep := config.Step{Type: config.DirectiveCopy, Src: "a", Dest: "b"}
	execStep := config.Step{Type: config.DirectiveExec, Argv: []string{"a", "b"}}
	if AutoKey(copyStep, "p", nil) == AutoKey(execStep, "p", nil) {
		t.Error("copy and exec with the same words collided")
	}
}

func TestExecKeyIgnoresArgvOrderlessQuoting(t *testing.T) {
	one := config.Step{Type: config.DirectiveExec, Argv: []string{"sh", "-c", "echo a b"}}
	two := config.Step{Type: config.DirectiveExec, Argv: []string{"sh", "-c", "echo a b"}}
	if AutoKey(one, "p", nil) != AutoKey(two, "p", nil) {
		t.Error("identical exec steps produced different keys")
	}
}

func TestManualKeyOverrides(t *testing.T) {
	a := ManualKey("deps-v1")
	if a != ManualKey("deps-v1") {
		t.Error("manual key is not deterministic")
	}
	if a == ManualKey("deps-v2") {
		t.Error("different manual keys collided")
	}
	auto := AutoKey(config.Step{Type: config.DirectiveCopy, Src: "x", Dest: "."}, "p",
		[]InputDigest{{Path: "x", Digest: "d"}})
	if a == auto {
		t.Error("manual key collided with an auto key")
	}
}

func TestHashInputsStableAndSensitive(t *testing.T) {
	ctx := t.TempDir()
	writeFile(t, filepath.Join(ctx, "a.txt"), "one")
	writeFile(t, filepath.Join(ctx, "nested", "b.txt"), "two")

	c := newTestCache(t)
	first, err := HashInputs(c, ctx, ".", 2)
	if err != nil {
		t.Fatalf("HashInputs: %v", err)
	}
	if len(first) != 2 {
		t.Fatalf("hashed %d files, want 2", len(first))
	}
	if first[0].Path > first[1].Path {
		t.Errorf("results are not sorted by path: %v", first)
	}

	second, err := HashInputs(c, ctx, ".", 2)
	if err != nil {
		t.Fatalf("HashInputs rerun: %v", err)
	}
	for i := range first {
		if first[i] != second[i] {
			t.Errorf("entry %d changed between runs: %v vs %v", i, first[i], second[i])
		}
	}

	writeFile(t, filepath.Join(ctx, "a.txt"), "one-modified")
	c2 := newTestCache(t)
	third, err := HashInputs(c2, ctx, ".", 2)
	if err != nil {
		t.Fatalf("HashInputs after edit: %v", err)
	}
	if third[0].Digest == first[0].Digest {
		t.Error("editing a file did not change its digest")
	}
}

func TestHashInputsSkipsSymlinks(t *testing.T) {
	ctx := t.TempDir()
	writeFile(t, filepath.Join(ctx, "real.txt"), "data")
	if err := os.Symlink("real.txt", filepath.Join(ctx, "link.txt")); err != nil {
		t.Skipf("symlinks unavailable: %v", err)
	}
	c := newTestCache(t)
	got, err := HashInputs(c, ctx, ".", 1)
	if err != nil {
		t.Fatalf("HashInputs: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("hashed %d entries, want 1 (symlinks carry no content)", len(got))
	}
}

func TestCacheStoreAndLookup(t *testing.T) {
	root := t.TempDir()
	c, err := NewCache(root, true)
	if err != nil {
		t.Fatalf("NewCache: %v", err)
	}

	upper := filepath.Join(t.TempDir(), "upper")
	writeFile(t, filepath.Join(upper, "marker"), "diff")

	if _, ok := c.Lookup("deadbeef"); ok {
		t.Error("empty cache reported a hit")
	}

	entry, err := c.Store("deadbeef", upper)
	if err != nil {
		t.Fatalf("Store: %v", err)
	}
	if _, err := os.Stat(filepath.Join(entry.Layer, "marker")); err != nil {
		t.Fatalf("stored layer missing its file: %v", err)
	}
	hit, ok := c.Lookup("deadbeef")
	if !ok {
		t.Fatal("stored key did not hit on lookup")
	}
	if hit.Layer != entry.Layer {
		t.Errorf("lookup returned %q, want %q", hit.Layer, entry.Layer)
	}
}

func TestCacheDisabled(t *testing.T) {
	root := t.TempDir()
	c, err := NewCache(root, false)
	if err != nil {
		t.Fatalf("NewCache: %v", err)
	}
	upper := filepath.Join(t.TempDir(), "upper")
	writeFile(t, filepath.Join(upper, "f"), "x")
	if _, err := c.Store("k", upper); err == nil {
		t.Error("Store succeeded on a disabled cache")
	}
	if _, ok := c.Lookup("k"); ok {
		t.Error("disabled cache reported a hit")
	}
}

func TestHashIndexPersists(t *testing.T) {
	root := t.TempDir()
	ctx := t.TempDir()
	writeFile(t, filepath.Join(ctx, "f"), "content")

	c, err := NewCache(root, true)
	if err != nil {
		t.Fatalf("NewCache: %v", err)
	}
	if _, err := HashInputs(c, ctx, ".", 1); err != nil {
		t.Fatalf("HashInputs: %v", err)
	}
	if err := c.SaveHashIndex(); err != nil {
		t.Fatalf("SaveHashIndex: %v", err)
	}

	reopened, err := NewCache(root, true)
	if err != nil {
		t.Fatalf("reopen NewCache: %v", err)
	}
	if len(reopened.hashing) == 0 {
		t.Error("hash index did not survive a reopen")
	}
}

func newTestCache(t *testing.T) *Cache {
	t.Helper()
	c, err := NewCache(t.TempDir(), true)
	if err != nil {
		t.Fatalf("NewCache: %v", err)
	}
	return c
}

func writeFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
}
