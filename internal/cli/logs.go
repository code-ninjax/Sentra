package cli

import (
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"time"

	"sentra/internal/runtime"
)

// LogsCmd implements `sentra logs CONTAINER [-f]`. Container stdout/stderr
// is captured to a log file at run time (attached runs stream to the
// terminal AND the file via runc; detached runs write the file only).
func LogsCmd(args []string) error {
	fs := flag.NewFlagSet("logs", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	follow := fs.Bool("f", false, "follow log output")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if fs.NArg() != 1 {
		return errors.New("usage: sentra logs CONTAINER [-f]")
	}

	c, err := runtime.LoadContainer(fs.Arg(0))
	if err != nil {
		return err
	}

	f, err := os.Open(c.LogFile)
	if err != nil {
		return fmt.Errorf("no logs for %q (attached runs print to the terminal)", c.ID)
	}
	defer f.Close()

	if _, err := io.Copy(os.Stdout, f); err != nil {
		return err
	}
	if !*follow {
		return nil
	}
	for {
		time.Sleep(500 * time.Millisecond)
		if _, err := io.Copy(os.Stdout, f); err != nil && err != io.EOF {
			return err
		}
	}
}
