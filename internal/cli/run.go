package cli

import (
	"errors"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"sentra/internal/build"
	"sentra/internal/runtime"
)

// mergeRuntimeEnv layers explicit -e pairs over the environment a built
// image recorded, and finally supplies the PATH defaults the runtime needs
// when neither source provided one.
func mergeRuntimeEnv(fromRecord, extra []string) []string {
	merged := make([]string, 0, len(fromRecord)+len(extra))
	merged = append(merged, fromRecord...)
	merged = append(merged, extra...)
	if !hasPATH(merged) {
		merged = append(merged,
			"PATH=/usr/local/sbin:/usr/local/bin:/usr/sbin:/usr/bin:/sbin:/bin",
			"HOME=/root",
			"TERM=xterm",
		)
	}
	return merged
}

func hasPATH(env []string) bool {
	for _, e := range env {
		if strings.HasPrefix(e, "PATH=") {
			return true
		}
	}
	return false
}

// RunCmd implements `sentra run --rootfs DIR` and `sentra run -t TAG`.
//
// MVP scope (workstream 1): a --rootfs path points at an unpacked rootfs —
// untarred by hand or produced by `sentra pull`. -t instead resolves a tag
// from a previous `sentra build`, which carries the rootfs, environment and
// entrypoint recorded at build time.
func RunCmd(args []string) error {
	fs := flag.NewFlagSet("run", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	rootfs := fs.String("rootfs", "", "path to an unpacked rootfs directory")
	tag := fs.String("t", "", "run a tag from a previous `sentra build -t TAG`")
	name := fs.String("name", "", "container ID (default: random hex)")
	memory := fs.Int64("memory", 0, "memory limit in MiB (0 = unlimited)")
	cpus := fs.Float64("cpus", 0, "CPU cores limit, e.g. 1.5 (0 = unlimited)")
	detach := fs.Bool("d", false, "run detached in the background")
	rw := fs.Bool("rw", false, "writable rootfs (read-only is the default)")
	envFlag := fs.String("e", "", "extra environment as KEY=VALUE (repeatable via commas)")

	if err := fs.Parse(args); err != nil {
		return err
	}

	explicit := fs.Args()
	cmd := explicit

	var (
		fromRecord []string
		extraEnv   []string
	)
	if *rootfs == "" && *tag != "" {
		img, err := build.Load(*tag)
		if err != nil {
			return err
		}
		*rootfs = img.Rootfs
		fromRecord = img.Env
		if len(explicit) == 0 {
			cmd = img.Entrypoint
		}
	}

	if *envFlag != "" {
		extraEnv = strings.Split(*envFlag, ",")
	}

	if len(cmd) == 0 {
		return errors.New(`no command given; usage: sentra run --rootfs DIR -- /bin/echo hello`)
	}
	if *rootfs == "" {
		return errors.New(`one of --rootfs or -t is required (e.g. sentra run -t myapp, or sentra run --rootfs DIR -- CMD)`)
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
		Env:      mergeRuntimeEnv(fromRecord, extraEnv),
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
