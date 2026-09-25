package build

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestCopyFileRegular(t *testing.T) {
	dir := t.TempDir()
	src := filepath.Join(dir, "src.txt")
	dst := filepath.Join(dir, "nested", "dst.txt")
	writeFile(t, src, "hello copy")

	if err := copyRegular(src, dst); err != nil {
		t.Fatalf("copyRegular: %v", err)
	}
	got, err := os.ReadFile(dst)
	if err != nil {
		t.Fatalf("read dst: %v", err)
	}
	if string(got) != "hello copy" {
		t.Errorf("content = %q, want %q", got, "hello copy")
	}
}

func TestCopyRegularZeroCopyPath(t *testing.T) {
	dir := t.TempDir()
	src := filepath.Join(dir, "big.bin")
	dst := filepath.Join(dir, "big.copy")

	// Larger than zeroCopyThreshold so the copy_file_range path is the
	// one under test; content must still arrive intact.
	payload := []byte(strings.Repeat("sentra", 32*1024))
	if err := os.WriteFile(src, payload, 0o644); err != nil {
		t.Fatal(err)
	}
	if int64(len(payload)) < zeroCopyThreshold {
		t.Fatalf("test payload is below the zero-copy threshold")
	}

	if err := copyRegular(src, dst); err != nil {
		t.Fatalf("copyRegular: %v", err)
	}
	got, err := os.ReadFile(dst)
	if err != nil {
		t.Fatalf("read dst: %v", err)
	}
	if len(got) != len(payload) {
		t.Fatalf("copied %d bytes, want %d", len(got), len(payload))
	}
	if string(got) != string(payload) {
		t.Error("zero-copy path corrupted the file")
	}
}

func TestCopyRegularPreservesExecutableBit(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("permission bits are not meaningful on Windows")
	}
	dir := t.TempDir()
	src := filepath.Join(dir, "run.sh")
	dst := filepath.Join(dir, "run.copy")
	writeFile(t, src, "#!/bin/sh\n")
	if err := os.Chmod(src, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := copyRegular(src, dst); err != nil {
		t.Fatalf("copyRegular: %v", err)
	}
	info, err := os.Stat(dst)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm()&0o111 == 0 {
		t.Errorf("mode = %v, executable bit was lost", info.Mode())
	}
}

func TestCopyIntoDirectoryContents(t *testing.T) {
	ctx := t.TempDir()
	writeFile(t, filepath.Join(ctx, "src", "one.txt"), "1")
	writeFile(t, filepath.Join(ctx, "src", "deep", "two.txt"), "2")

	root := filepath.Join(t.TempDir(), "root")
	if err := os.MkdirAll(root, 0o755); err != nil {
		t.Fatal(err)
	}

	// COPY of a directory copies its contents into the destination.
	if err := copyInto(ctx, root, "src", "dst"); err != nil {
		t.Fatalf("copyInto: %v", err)
	}
	if _, err := os.Stat(filepath.Join(root, "dst", "one.txt")); err != nil {
		t.Errorf("dst/one.txt missing: %v", err)
	}
	if _, err := os.Stat(filepath.Join(root, "dst", "deep", "two.txt")); err != nil {
		t.Errorf("dst/deep/two.txt missing: %v", err)
	}
}

func TestCopyIntoFileKeepsNameInDirectoryDest(t *testing.T) {
	ctx := t.TempDir()
	writeFile(t, filepath.Join(ctx, "app.txt"), "content")
	root := filepath.Join(t.TempDir(), "root")

	// A destination naming an existing directory receives the file under
	// its own name, matching Docker's COPY semantics.
	if err := os.MkdirAll(filepath.Join(root, "target"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := copyInto(ctx, root, "app.txt", "target"); err != nil {
		t.Fatalf("copyInto: %v", err)
	}
	if _, err := os.Stat(filepath.Join(root, "target", "app.txt")); err != nil {
		t.Errorf("target/app.txt missing: %v", err)
	}
}

func TestCopyIntoSymlink(t *testing.T) {
	ctx := t.TempDir()
	writeFile(t, filepath.Join(ctx, "real"), "data")
	if err := os.Symlink("real", filepath.Join(ctx, "link")); err != nil {
		t.Skipf("symlinks unavailable: %v", err)
	}
	root := filepath.Join(t.TempDir(), "root")
	if err := copyInto(ctx, root, "link", "copied"); err != nil {
		t.Fatalf("copyInto: %v", err)
	}
	target, err := os.Readlink(filepath.Join(root, "copied"))
	if err != nil {
		t.Fatalf("copied entry is not a symlink: %v", err)
	}
	if target != "real" {
		t.Errorf("symlink target = %q, want %q", target, "real")
	}
}

func TestCopyIntoRejectsEscapingDestination(t *testing.T) {
	ctx := t.TempDir()
	writeFile(t, filepath.Join(ctx, "f"), "x")
	root := filepath.Join(t.TempDir(), "root")
	if err := os.MkdirAll(root, 0o755); err != nil {
		t.Fatal(err)
	}

	if err := copyInto(root, ctx, filepath.Join(ctx, "f"), "../../escape"); err == nil {
		t.Fatal("copyInto allowed a destination outside the build root")
	}
}

func TestCopyIntoMissingSource(t *testing.T) {
	ctx := t.TempDir()
	root := filepath.Join(t.TempDir(), "root")
	if err := copyInto(ctx, root, "nope", "dest"); err == nil {
		t.Fatal("copyInto accepted a missing source")
	}
}

func TestCopyIntoAnchorsAbsoluteSource(t *testing.T) {
	ctx := t.TempDir()
	writeFile(t, filepath.Join(ctx, "out", "binary"), "compiled")
	root := filepath.Join(t.TempDir(), "root")

	// `copy --from=builder /out/binary /app/binary` uses an absolute source,
	// which is resolved inside the source root, never on the host.
	if err := copyInto(ctx, root, "/out/binary", "/app/binary"); err != nil {
		t.Fatalf("copyInto: %v", err)
	}
	if _, err := os.Stat(filepath.Join(root, "app", "binary")); err != nil {
		t.Errorf("app/binary missing: %v", err)
	}
}

func TestCopyIntoRejectsTraversalSource(t *testing.T) {
	ctx := t.TempDir()
	writeFile(t, filepath.Join(t.TempDir(), "secret"), "nope")
	root := filepath.Join(t.TempDir(), "root")
	if err := copyInto(ctx, root, "../secret", "dest"); err == nil {
		t.Fatal("copyInto allowed a source outside the build context")
	}
}
