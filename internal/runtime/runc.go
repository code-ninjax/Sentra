package runtime

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
)

// Runc wraps invocations of the external `runc` binary.
//
// Design note: we exec `runc` instead of linking libcontainer's Go API.
// libcontainer pulls in megabytes of dependencies and couples us to its
// internal privilege model; shelling out keeps the Sentra binary small,
// stays daemonless (we are the parent process), and lets runc own the
// hairy kernel details it was built for.
type Runc struct {
	Binary   string // resolved via PATH when empty
	Rootless bool   // auto-detected; passes --rootless=true to runc
}

func NewRunc() *Runc {
	return &Runc{
		Binary:   "runc",
		Rootless: os.Getuid() != 0,
	}
}

func (r *Runc) args(extra ...string) []string {
	out := []string{}
	if r.Rootless {
		out = append(out, "--rootless=true")
	}
	return append(out, extra...)
}

func (r *Runc) command(stdout, stderr io.Writer, arg ...string) *exec.Cmd {
	cmd := exec.Command(r.Binary, r.args(arg...)...)
	if stdout != nil {
		cmd.Stdout = stdout
	}
	if stderr != nil {
		cmd.Stderr = stderr
	}
	return cmd
}

// Run executes `runc run --bundle <bundle> <id>` and blocks until the
// container exits. Output is streamed to the provided writers.
// Returns the container's exit code.
func (r *Runc) Run(id, bundle string, stdout, stderr io.Writer) (int, error) {
	err := r.command(stdout, stderr, "run", "--bundle", bundle, id).Run()
	return exitCode(err), err
}

// Detach starts the container in the background (`runc create` + `start`),
// redirecting container output into logFile so `sentra logs` can read it.
func (r *Runc) Detach(id, bundle, logFile string) error {
	f, err := os.OpenFile(logFile, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o644)
	if err != nil {
		return fmt.Errorf("open container log: %w", err)
	}
	defer f.Close()

	create := r.command(f, f, "create", "--bundle", bundle, "--pid-file", pidFile(bundle), id)
	if err := create.Run(); err != nil {
		return fmt.Errorf("runc create: %w", err)
	}
	if err := r.command(nil, nil, "start", id).Run(); err != nil {
		return fmt.Errorf("runc start: %w", err)
	}
	return nil
}

// Kill sends sig to the container's init process.
func (r *Runc) Kill(id, sig string) error {
	return r.command(nil, nil, "kill", id, sig).Run()
}

// Delete tears down the container, forcing it if still alive.
func (r *Runc) Delete(id string) error {
	return r.command(nil, nil, "delete", "-f", id).Run()
}

// State queries runc for the live container state. Returns an error when
// the container does not exist in runc's view.
func (r *Runc) State(id string) (*RuncState, error) {
	var buf bytes.Buffer
	if err := r.command(&buf, nil, "state", id).Run(); err != nil {
		return nil, err
	}
	st := &RuncState{}
	if err := json.Unmarshal(buf.Bytes(), st); err != nil {
		return nil, fmt.Errorf("parse runc state: %w", err)
	}
	return st, nil
}

// Exec runs argv inside a running container, wiring our stdio through.
func (r *Runc) Exec(id string, stdin io.Reader, stdout, stderr io.Writer, argv []string) error {
	args := append([]string{"exec", id}, argv...)
	cmd := r.command(stdout, stderr, args...)
	cmd.Stdin = stdin
	return cmd.Run()
}

type RuncState struct {
	ID     string `json:"id"`
	Status string `json:"status"`
	Pid    int    `json:"pid"`
	Bundle string `json:"bundle"`
}

func pidFile(bundle string) string { return bundle + "/pid" }

func exitCode(err error) int {
	if err == nil {
		return 0
	}
	var ee *exec.ExitError
	if errors.As(err, &ee) {
		return ee.ExitCode()
	}
	return -1
}
