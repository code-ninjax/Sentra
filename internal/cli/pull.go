package cli

import (
	"errors"
	"flag"
	"fmt"
	"os"

	"sentra/internal/image"
)

// PullCmd implements `sentra pull <image>`: fetch manifest + layers from
// Docker Hub / GHCR into the content-addressable store and produce a
// merged rootfs ready for `sentra run --rootfs <path>`.
func PullCmd(args []string) error {
	fs := flag.NewFlagSet("pull", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	if err := fs.Parse(args); err != nil {
		return err
	}
	if fs.NArg() != 1 {
		return errors.New("usage: sentra pull IMAGE (e.g. sentra pull alpine:latest)")
	}

	rootfs, err := image.Pull(fs.Arg(0))
	if err != nil {
		return err
	}
	fmt.Printf("\nrun it:\n  sudo ./bin/sentra-linux run --rootfs %s -- /bin/echo hello\n", rootfs)
	return nil
}
