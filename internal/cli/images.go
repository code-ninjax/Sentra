package cli

import (
	"errors"
	"flag"
	"fmt"
	"os"
	"strings"
	"text/tabwriter"

	"sentra/internal/build"
)

// ImagesCmd implements `sentra images`: list the tags produced by
// `sentra build`, and `sentra images -r TAG` to forget one.
func ImagesCmd(args []string) error {
	fs := flag.NewFlagSet("images", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	remove := fs.String("r", "", "remove a built image tag")
	if err := fs.Parse(args); err != nil {
		return err
	}

	if *remove != "" {
		if err := build.Remove(*remove); err != nil {
			return err
		}
		fmt.Println(*remove)
		return nil
	}
	if fs.NArg() > 0 {
		return errors.New("usage: sentra images [-r TAG]")
	}

	tags, err := build.List()
	if err != nil {
		return err
	}
	if len(tags) == 0 {
		fmt.Println("no built images (run: sentra build -t myapp .)")
		return nil
	}

	w := tabwriter.NewWriter(os.Stdout, 0, 4, 2, ' ', 0)
	defer w.Flush()
	fmt.Fprintln(w, "TAG\tROOTFS\tENTRYPOINT")
	for _, tag := range tags {
		img, err := build.Load(tag)
		if err != nil {
			// A record whose rootfs is gone still deserves a row; show
			// what happened rather than hiding the tag.
			fmt.Fprintf(w, "%s\t<unavailable: %s>\t\n", tag, err)
			continue
		}
		entry := strings.Join(img.Entrypoint, " ")
		if entry == "" {
			entry = "-"
		}
		fmt.Fprintf(w, "%s\t%s\t%s\n", tag, img.Rootfs, entry)
	}
	return nil
}
