package cli

import (
	"fmt"
	stdruntime "runtime"
)

// Version is the release tag (v0.1.0, ...). Release builds stamp it via:
//   go build -ldflags "-X sentra/internal/cli.Version=$(git describe --tags)"
// Local builds report "dev".
var Version = "dev"

// VersionCmd implements `sentra version` (and --version): one line,
// script-friendly, no daemon to ask.
func VersionCmd() error {
	fmt.Printf("sentra version %s (%s/%s)\n", Version, stdruntime.GOOS, stdruntime.GOARCH)
	return nil
}
