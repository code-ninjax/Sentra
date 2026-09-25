//go:build !linux

package build

import (
	"errors"
	"os"
)

// copyFileRange has no equivalent outside Linux. Reporting an error
// makes copyRegular use its buffered path, which is correct everywhere.
func copyFileRange(src, dst string, mode os.FileMode) error {
	_ = src
	_ = dst
	_ = mode
	return errors.New("copy_file_range is Linux-only")
}
