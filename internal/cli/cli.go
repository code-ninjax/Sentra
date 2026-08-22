package cli

import (
	"errors"
	"os/exec"
)

// ExitError carries a container's exit code up to main() so `sentra run`
// and `sentra exec` can exit with the same code the process inside the
// container did.
type ExitError struct{ Code int }

func (e *ExitError) Error() string {
	return ""
}

// exitCodeOf reports the process exit code inside err, or -1 when err is
// not an exit-status error (e.g. runc failed to start).
func exitCodeOf(err error) int {
	if err == nil {
		return 0
	}
	var ee *exec.ExitError
	if errors.As(err, &ee) {
		if code := ee.ExitCode(); code >= 0 {
			return code
		}
	}
	return -1
}
