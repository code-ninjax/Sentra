package cli

import (
	"errors"
	"flag"
	"fmt"
	"os"

	"sentra/internal/runtime"
)

// RmCmd implements `sentra rm CONTAINER`: removes the state record, bundle,
// and runc bookkeeping. Refuses running containers unless --force.
func RmCmd(args []string) error {
	fs := flag.NewFlagSet("rm", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	force := fs.Bool("f", false, "force-remove a running container")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if fs.NArg() != 1 {
		return errors.New("usage: sentra rm CONTAINER")
	}

	id := fs.Arg(0)
	if _, err := runtime.LoadContainer(id); err != nil {
		return err
	}

	r := runtime.NewRunc()
	if _, err := r.State(id); err == nil {
		if !*force {
			return fmt.Errorf("container %q is running; use -f to force", id)
		}
		if err := r.Delete(id); err != nil {
			return fmt.Errorf("force-delete %q: %w", id, err)
		}
	}

	if err := runtime.RemoveContainer(id); err != nil {
		return err
	}
	fmt.Println(id)
	return nil
}
