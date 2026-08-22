package cli

import (
	"errors"
	"flag"
	"fmt"

	"sentra/internal/runtime"
)

// StopCmd implements `sentra stop CONTAINER`: SIGTERM, then SIGKILL after
// a grace period handled by runc's kill semantics.
func StopCmd(args []string) error {
	fs := flag.NewFlagSet("stop", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	sig := fs.String("signal", "SIGTERM", "signal to send")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if fs.NArg() != 1 {
		return errors.New("usage: sentra stop CONTAINER")
	}

	id := fs.Arg(0)
	c, err := runtime.LoadContainer(id)
	if err != nil {
		return err
	}

	r := runtime.NewRunc()
	if _, err := r.State(id); err != nil {
		return fmt.Errorf("container %q is not running", id)
	}
	if err := r.Kill(id, *sig); err != nil {
		return fmt.Errorf("stop %q: %w", id, err)
	}

	c.Status = "stopped"
	return runtime.SaveContainer(c)
}
