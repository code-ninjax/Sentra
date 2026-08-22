package main

import (
	"errors"
	"fmt"
	"os"

	"sentra/internal/cli"
)

const usage = `sentra — daemonless OCI container runtime

Usage:
  sentra run  --rootfs DIR [--rw] [-d] [--name ID] [--memory MB] [--cpus N] -- CMD [ARGS...]
  sentra ps
  sentra stop CONTAINER
  sentra rm   CONTAINER
  sentra logs CONTAINER [-f]
  sentra exec CONTAINER -- CMD [ARGS...]
`

func main() {
	if len(os.Args) < 2 {
		fmt.Fprint(os.Stderr, usage)
		os.Exit(2)
	}

	var err error
	switch os.Args[1] {
	case "run":
		err = cli.RunCmd(os.Args[2:])
	case "ps":
		err = cli.PsCmd(os.Args[2:])
	case "stop":
		err = cli.StopCmd(os.Args[2:])
	case "rm":
		err = cli.RmCmd(os.Args[2:])
	case "logs":
		err = cli.LogsCmd(os.Args[2:])
	case "exec":
		err = cli.ExecCmd(os.Args[2:])
	default:
		fmt.Fprintf(os.Stderr, "unknown command %q\n\n%s", os.Args[1], usage)
		os.Exit(2)
	}

	if err != nil {
		var ec *cli.ExitError
		if errors.As(err, &ec) {
			os.Exit(ec.Code)
		}
		fmt.Fprintln(os.Stderr, "sentra:", err)
		os.Exit(1)
	}
}
