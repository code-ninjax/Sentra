package cli

import (
	"errors"
	"flag"
	"os"

	"sentra/internal/runtime"
)

// ExecCmd implements `sentra exec CONTAINER -- CMD [ARGS...]`: runs argv
// inside a running container with our stdio wired through, exiting with
// the in-container process's exit code.
func ExecCmd(args []string) error {
	fs := flag.NewFlagSet("exec", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	if err := fs.Parse(args); err != nil {
		return err
	}
	argv := fs.Args()
	if len(argv) < 2 {
		return errors.New("usage: sentra exec CONTAINER -- CMD [ARGS...]")
	}

	id := argv[0]
	c, err := runtime.LoadContainer(id)
	if err != nil {
		return err
	}
	_ = c // existence check; runc is the source of truth for running state

	r := runtime.NewRunc()
	if err := r.Exec(id, os.Stdin, os.Stdout, os.Stderr, argv[1:]); err != nil {
		code := exitCodeOf(err)
		if code >= 0 {
			return &ExitError{Code: code}
		}
		return err
	}
	return nil
}
