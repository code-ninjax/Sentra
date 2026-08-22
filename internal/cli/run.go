package cli

import (
	"errors"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"sentra/internal/runtime"
)

// RunCmd implements `sentra run --rootfs DIR [--flags] -- CMD [ARGS...]`.
//
// MVP scope (workstream 1): the rootfs must already exist on disk — either
// untarred by hand or produced by workstream 2's `sentra pull`. Image
// resolution by name is deliberately not wired yet.
func RunCmd(args []string) error {
	fs := flag.NewFlagSet("run", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	rootfs := fs.String("rootfs", "", "path to an unpacked rootfs directory (required)")
	name := fs.String("name", "", "container ID (default: random hex)")
	memory := fs.Int64("memory", 0, "memory limit in MiB (0 = unlimited)")
	cpus := fs.Float64("cpus", 0, "CPU cores limit, e.g. 1.5 (0 = unlimited)")
	detach := fs.Bool("d", false, "run detached in the background")
	rw := fs.Bool("rw", false, "writable rootfs (read-only is the default)")

	if err := fs.Parse(args); err != nil {
		return err
	}

	cmd := fs.Args()
	if len(cmd) == 0 {
		return errors.New(`no command given; usage: sentra run --rootfs DIR -- /bin/echo hello`)
	}
	if *rootfs == "" {
		return errors.New("--rootfs is required (image pulling arrives in workstream 2)")
	}
	info, err := os.Stat(*rootfs)
	if err != nil {
		return fmt.Errorf("rootfs %q: %w", *rootfs, err)
	}
	if !info.IsDir() {
		return fmt.Errorf("rootfs %q is not a directory", *rootfs)
	}

	id := *name
	if id == "" {
		if id, err = runtime.NewID(); err != nil {
			return err
		}
	}

	limits := runtime.DefaultLimits()
	if *memory > 0 {
		limits.MemoryLimitBytes = *memory * 1024 * 1024
	}
	if *cpus > 0 {
		limits.CPUQuota = int64(*cpus * 100000)
	}

	rootfsAbs, err := filepath.Abs(*rootfs)
	if err != nil {
		return err
	}
	c := &runtime.Container{
		ID:       id,
		Rootfs:   rootfsAbs,
		Cmd:      cmd,
		Status:   "creating",
		Readonly: !*rw,
		Detached: *detach,
	}
	if err := runtime.CreateBundle(c, limits); err != nil {
		return err
	}
	if err := runtime.SaveContainer(c); err != nil {
		return err
	}

	r := runtime.NewRunc()

	if *detach {
		if err := r.Detach(id, c.Bundle, c.LogFile); err != nil {
			c.Status = "error"
			_ = runtime.SaveContainer(c)
			return err
		}
		c.Status = "running"
		if st, err := r.State(id); err == nil {
			c.Pid = st.Pid
		}
		_ = runtime.SaveContainer(c)
		fmt.Println(id)
		return nil
	}

	code, runErr := r.Run(id, c.Bundle, os.Stdout, os.Stderr)

	c.Status = "exited"
	c.ExitCode = code
	_ = runtime.SaveContainer(c)
	_ = r.Delete(id) // best-effort teardown of runc bookkeeping

	if code < 0 && runErr != nil {
		return fmt.Errorf("runc: %w", runErr)
	}
	return &ExitError{Code: code}
}

// truncate renders cmd slices compactly for ps output.
func truncate(argv []string, n int) string {
	s := strings.Join(argv, " ")
	if len(s) > n {
		return s[:n-1] + "…"
	}
	return s
}
