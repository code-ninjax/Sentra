package cli

import (
	"errors"
	"flag"
	"fmt"
	"os"
	"strings"
	"time"

	"sentra/internal/build"
)

// BuildCmd implements `sentra build -t TAG [CONTEXT]`: run a Sentrafile
// and produce a runnable image rootfs.
//
//	--no-cache    re-run every step, ignoring cached layers
//	--workers N   parallel step workers (default: cores, capped at 4)
//	--file PATH   Sentrafile location (default: CONTEXT/Sentrafile)
func BuildCmd(args []string) error {
	fs := flag.NewFlagSet("build", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	tag := fs.String("t", "", "image tag (e.g. myapp)")
	noCache := fs.Bool("no-cache", false, "ignore cached layers and re-run every step")
	workers := fs.Int("workers", 0, "parallel step workers (0 = auto, 2-4)")
	file := fs.String("file", "", "path to the Sentrafile (default: CONTEXT/Sentrafile)")
	if err := fs.Parse(args); err != nil {
		return err
	}

	context := "."
	if fs.NArg() > 0 {
		context = fs.Arg(0)
	}
	if *tag == "" {
		return errors.New("a tag is required: sentra build -t myapp .")
	}
	if info, err := os.Stat(context); err != nil || !info.IsDir() {
		return fmt.Errorf("build context %q is not a directory", context)
	}

	res, err := build.Build(build.Options{
		ContextDir: context,
		Sentrafile: *file,
		Tag:        *tag,
		NoCache:    *noCache,
		Workers:    *workers,
		Out:        os.Stdout,
	})
	if err != nil {
		return err
	}

	hits, total := 0, 0
	for _, s := range res.Steps {
		if s.CacheHit {
			hits++
		}
		if s.Directive == "copy" || s.Directive == "exec" {
			total++
		}
	}

	fmt.Printf("\nbuilt %s in %s (%d/%d layers cached)\n", res.Tag, res.Duration.Round(time.Millisecond), hits, total)
	fmt.Printf("rootfs: %s\n", res.Rootfs)

	cmd := []string{"sudo", "./bin/sentra-linux", "run", "--rootfs", res.Rootfs, "--"}
	cmd = append(cmd, res.Entrypoint...)
	if len(res.Entrypoint) == 0 {
		cmd = append(cmd, "/bin/sh")
	}
	fmt.Printf("run it:\n  %s\n", strings.Join(cmd, " "))
	if len(res.Expose) > 0 {
		fmt.Printf("exposes: %s (host publishing arrives with the networking workstream)\n",
			strings.Join(res.Expose, " "))
	}
	return nil
}
