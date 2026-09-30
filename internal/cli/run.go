package cli

import (
	"errors"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"sentra/internal/build"
	"sentra/internal/network"
	"sentra/internal/runtime"
	"sentra/internal/security"
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
	securityFlag := fs.String("security", "", "seccomp level: default, strict or unconfined")
	rootlessFlag := fs.Bool("rootless", true, "run in a user namespace (rootless by default)")
	envFlag := fs.String("e", "", "extra environment as KEY=VALUE (repeatable via commas)")
	publishFlag := fs.String("p", "", "publish ports, comma separated: 8080, 8080:80, 8080:80/udp")
	noPublish := fs.Bool("no-publish", false, "do not publish the ports a built image exposed")

	if err := fs.Parse(args); err != nil {
		return err
	}

	explicit := fs.Args()
	cmd := explicit

	// Sentra's baseline, then the posture a Sentrafile declared at build
	// time, then explicit flags. That ordering is what makes `run -t` pick
	// up the image's own security declaration without the user restating it.
	posture := security.Default()
	var (
		fromRecord []string
		extraEnv   []string
		fromExpose []string
	)
	if *rootfs == "" && *tag != "" {
		img, err := build.Load(*tag)
		if err != nil {
			return err
		}
		*rootfs = img.Rootfs
		fromRecord = img.Env
		fromExpose = img.Expose
		if len(explicit) == 0 {
			cmd = img.Entrypoint
		}
		posture = security.Posture{
			Rootless: img.Security.Rootless,
			Readonly: img.Security.Readonly,
			Seccomp:  security.Level(img.Security.Seccomp),
		}
		if posture.Seccomp == "" {
			posture.Seccomp = security.LevelDefault
		}
	}

	if *rw {
		posture.Readonly = false
	}
	if !*rootlessFlag {
		posture.Rootless = false
	}
	if *securityFlag != "" {
		level := security.Level(*securityFlag)
		if !level.Valid() {
			return fmt.Errorf("unknown --security level %q (want default, strict or unconfined)", *securityFlag)
		}
		posture.Seccomp = level
	}

	if *envFlag != "" {
		extraEnv = strings.Split(*envFlag, ",")
	}

	var publishSpecs []string
	if *publishFlag != "" {
		publishSpecs = strings.Split(*publishFlag, ",")
	}

	// Report the posture on stderr so it never contaminates a container's
	// piped stdout, and so a posture inherited from a Sentrafile's secure
	// directive is visible rather than implicit.
	//
	// Rootless is only possible when we are not already root, so the
	// reported posture is clamped the same way runtime.BuildSpec clamps it.
	// Reporting what was requested rather than what ran would be a lie in a
	// security feature.
	if os.Geteuid() == 0 {
		posture.Rootless = false
	}
	fmt.Fprintf(os.Stderr, "sentra: %s\n", posture.Summary())

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

	publish, err := resolvePublish(publishSpecs, fromExpose, *noPublish)
	if err != nil {
		return err
	}

	c := &runtime.Container{
		ID:       id,
		Rootfs:   rootfsAbs,
		Cmd:      cmd,
		Status:   "creating",
		Readonly: posture.Readonly,
		Detached: *detach,
		Env:      mergeRuntimeEnv(fromRecord, extraEnv),
		Posture:  posture,
		Publish:  portSpecs(publish),
	}

	// Networking is set up before the container exists: the namespace has to
	// be ready to join, and the veth must be moved into it while nothing is
	// using it. Rootless runs keep an isolated namespace with loopback only,
	// which the note on stderr explains.
	netCfg, teardown, err := setupNetwork(c, publish)
	if err != nil {
		return err
	}

	if err := runtime.CreateBundle(c, limits); err != nil {
		teardown()
		return err
	}
	if err := runtime.SaveContainer(c); err != nil {
		teardown()
		return err
	}

	r := runtime.NewRunc()

	if *detach {
		if err := r.Detach(id, c.Bundle, c.LogFile); err != nil {
			c.Status = "error"
			_ = runtime.SaveContainer(c)
			teardown()
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

	// An attached container is gone once runc returns, so its networking
	// goes with it. A detached one keeps its namespace until stop or rm.
	teardown()
	_ = netCfg

	c.Status = "exited"
	c.ExitCode = code
	_ = runtime.SaveContainer(c)
	_ = r.Delete(id) // best-effort teardown of runc bookkeeping

	if code < 0 && runErr != nil {
		return fmt.Errorf("runc: %w", runErr)
	}
	return &ExitError{Code: code}
}

// resolvePublish decides which ports to open: explicit -p mappings win, and
// otherwise the ports a built image declared with `expose` are published as
// themselves. Docker requires -p every time; reading the declaration from
// the image is the behaviour a Sentrafile's `expose` implies, and it is
// announced on stderr rather than done silently.
func resolvePublish(explicit []string, exposed []string, disabled bool) ([]network.PortMap, error) {
	if disabled {
		return nil, nil
	}
	if len(explicit) > 0 {
		return network.ParsePortMaps(explicit)
	}
	if len(exposed) == 0 {
		return nil, nil
	}
	return network.ParsePortMaps(exposed)
}

// portSpecs renders mappings back into their short form so container state
// can reconstruct them for teardown.
func portSpecs(ports []network.PortMap) []string {
	if len(ports) == 0 {
		return nil
	}
	out := make([]string, 0, len(ports))
	for _, p := range ports {
		out = append(out, p.String())
	}
	return out
}

// setupNetwork prepares the container's network namespace and returns a
// teardown function plus the config teardown will need.
func setupNetwork(c *runtime.Container, publish []network.PortMap) (network.Config, func(), error) {
	cfg := network.Config{
		ID:       c.ID,
		Publish:  publish,
		StateDir: stateRoot(),
		Out:      os.Stderr,
	}

	if os.Geteuid() != 0 {
		fmt.Fprintf(os.Stderr, "sentra: container networking needs root; this container gets loopback only\n")
		fmt.Fprintf(os.Stderr, "        (run with sudo for outbound networking and -p)\n")
		// Detached rootless containers still need their state torn down.
		return cfg, func() {}, nil
	}

	netns, err := network.Setup(cfg)
	if err != nil {
		return cfg, func() {}, err
	}
	c.NetnsPath = netns

	if len(publish) > 0 {
		parts := make([]string, 0, len(publish))
		for _, p := range publish {
			parts = append(parts, p.String())
		}
		fmt.Fprintf(os.Stderr, "sentra: published %s\n", strings.Join(parts, ", "))
	}

	return cfg, func() {
		if errs := network.Teardown(cfg); len(errs) > 0 {
			for _, err := range errs {
				fmt.Fprintf(os.Stderr, "sentra: network cleanup: %v\n", err)
			}
		}
	}, nil
}

// truncate renders cmd slices compactly for ps output.
func truncate(argv []string, n int) string {
	s := strings.Join(argv, " ")
	if len(s) > n {
		return s[:n-1] + "…"
	}
	return s
}

// stateRoot is the resolved Sentra state directory, or empty when it cannot
// be determined. Callers that need it strictly should use runtime.StateRoot
// directly.
func stateRoot() string {
	dir, err := runtime.StateRoot()
	if err != nil {
		return ""
	}
	return dir
}
