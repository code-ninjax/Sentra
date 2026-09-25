package cli

import (
	"errors"
	"flag"
	"fmt"
	"os"

	"sentra/internal/build"
)

// PushCmd implements `sentra push TAG [DEST]`: upload a built image to an
// OCI registry. DEST defaults to the build tag, e.g. `sentra push demo`
// targets docker.io/library/demo.
//
// Only this build's layers are uploaded — the base image is referenced from
// the registry it was pulled from, so an unchanged rebuild pushes almost
// nothing. Set SENTRA_REGISTRY_TOKEN when the registry requires auth.
func PushCmd(args []string) error {
	fs := flag.NewFlagSet("push", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	if err := fs.Parse(args); err != nil {
		return err
	}
	if fs.NArg() < 1 {
		return errors.New("usage: sentra push TAG [DEST]")
	}

	tag := fs.Arg(0)
	dest := tag
	if fs.NArg() > 1 {
		dest = fs.Arg(1)
	}

	res, err := build.PushArtifact(tag, dest)
	if err != nil {
		return err
	}
	fmt.Printf("pushed %s\n", res.Reference)
	fmt.Printf("digest %s (%d build layer(s))\n", res.Digest, res.Layers)
	return nil
}
