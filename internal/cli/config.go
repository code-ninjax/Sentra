package cli

import (
	"errors"
	"flag"
	"fmt"
	"os"
	"text/tabwriter"

	"sentra/internal/config"
)

// ConfigCmd implements `sentra config FILE`: parses a Sentrafile and
// prints the resulting build plan. This is the parser's inspection/test
// surface — the build engine that executes these steps is workstream 4.
func ConfigCmd(args []string) error {
	fs := flag.NewFlagSet("config", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	quiet := fs.Bool("q", false, "validate only, print nothing")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if fs.NArg() != 1 {
		return errors.New("usage: sentra config FILE")
	}

	plan, err := config.ParseFile(fs.Arg(0))
	if err != nil {
		return err
	}
	if *quiet {
		fmt.Println("ok")
		return nil
	}

	w := tabwriter.NewWriter(os.Stdout, 0, 2, 2, ' ', 0)
	defer w.Flush()
	fmt.Fprintf(w, "stages\t%d\n", len(plan.Stages))
	fmt.Fprintf(w, "secure\trootless=%t readonly=%t seccomp=%s\n",
		plan.Security.Rootless, plan.Security.Readonly, plan.Security.Seccomp)

	for i, stage := range plan.Stages {
		last := i == len(plan.Stages)-1
		name := stage.Name
		if name == "" {
			name = "(final)"
		}
		role := "stage"
		if last {
			role = "image"
		}
		fmt.Fprintf(w, "\n%s\t%s\tbase %s\n", role, name, stage.Base)
		fmt.Fprintf(w, "  steps\t%d\n", len(stage.Steps))
		for _, s := range stage.Steps {
			line := fmt.Sprintf("  %d\t%s\t%s", s.Line, s.Type, s.Summary())
			if s.From != "" {
				line += fmt.Sprintf("\t[from: %s]", s.From)
			}
			if s.Cache != "" {
				line += fmt.Sprintf("\t[cache: %s]", s.Cache)
			}
			fmt.Fprintln(w, line)
		}
	}
	return nil
}
