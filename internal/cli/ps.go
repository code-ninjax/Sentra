package cli

import (
	"flag"
	"fmt"
	"os"
	"text/tabwriter"

	"sentra/internal/runtime"
)

// PsCmd implements `sentra ps`. Reads Sentra's state store and refreshes
// each entry against runc's live view when possible.
func PsCmd(args []string) error {
	fs := flag.NewFlagSet("ps", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	all := fs.Bool("a", true, "show stopped containers too")
	if err := fs.Parse(args); err != nil {
		return err
	}

	containers, err := runtime.ListContainers()
	if err != nil {
		return err
	}
	if len(containers) == 0 {
		fmt.Println("no containers")
		return nil
	}

	r := runtime.NewRunc()
	w := tabwriter.NewWriter(os.Stdout, 0, 4, 2, ' ', 0)
	defer w.Flush()
	fmt.Fprintln(w, "ID\tSTATUS\tCMD\tCREATED")

	for _, c := range containers {
		status := c.Status
		if st, err := r.State(c.ID); err == nil {
			status = st.Status // authoritative while runc knows the container
			if status != c.Status {
				c.Status = status
				_ = runtime.SaveContainer(c)
			}
		} else if status == "running" || status == "creating" {
			status = "gone"
			c.Status = status
			_ = runtime.SaveContainer(c)
		}
		if !*all && (status == "exited" || status == "stopped" || status == "gone") {
			continue
		}
		fmt.Fprintf(w, "%s\t%s\t%s\t%s\n",
			c.ID, status, truncate(c.Cmd, 40), c.CreatedAt.Format("2006-01-02 15:04"))
	}
	return nil
}
