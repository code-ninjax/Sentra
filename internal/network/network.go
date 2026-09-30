// Package network gives Sentra containers real networking: a bridge with
// outbound NAT, and published ports from the host.
//
// There is exactly one implementation, on purpose. Podman's driver zoo —
// bridge, macvlan, overlay, slirp, pasta, each with its own configuration
// surface — is the thing this package refuses to become. A container gets a
// veth on the sentra0 bridge, an address from one subnet, and NAT to the
// outside. If it needs something else, it does not need Sentra.
//
// The host tools (`ip`, `iptables`) are invoked rather than reimplemented
// over netlink. That is the same choice the runtime makes with runc: the
// kernel-facing plumbing already has a battle-tested front end, and shelling
// out keeps the binary free of netlink machinery and keeps failures legible.
// The commands are built as data and unit-tested without root.
//
// Rootless containers keep an isolated network namespace with loopback only.
// A bridge needs root to create, and user-space networking (slirp4netns,
// pasta) is a deliberate follow-up rather than something half-built here.
package network

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
)

// Config describes the network a container should join.
type Config struct {
	// ID names the container, and derives its namespace and veth names.
	ID string
	// Publish are host-to-container port mappings.
	Publish []PortMap
	// StateDir holds allocated addresses.
	StateDir string
	// Out receives progress and warnings.
	Out Writer
	// DryRun builds the command plan without executing anything. Tests use
	// it to assert on rules without touching the host.
	DryRun bool
}

// Writer is the subset of io.Writer this package needs.
type Writer interface {
	Write(p []byte) (int, error)
}

// Setup wires a container into the network and returns the netns path the
// runtime spec should use.
//
// The sequence is the conventional one, and order matters: the netns must
// exist before runc starts the container, because runc joins an existing
// namespace rather than creating one we can attach to afterwards.
//
//	ip netns add sentra-<id>
//	veth pair, container end moved into the namespace
//	bridge end enslaved to sentra0
//	address, loopback and default route set inside
//	MASQUERADE for the subnet, DNAT for each published port
func Setup(cfg Config) (netnsPath string, err error) {
	if cfg.ID == "" {
		return "", fmt.Errorf("network: container id is required")
	}
	if err := requireRoot(); err != nil {
		return "", err
	}
	if err := requireTools(); err != nil {
		return "", err
	}

	nsName := nsNameFor(cfg.ID)
	netnsPath = filepath.Join(netnsDir, nsName)

	alloc, err := Allocate(cfg.StateDir, cfg.ID)
	if err != nil {
		return "", err
	}

	if err := EnsureBridge(cfg); err != nil {
		return "", err
	}

	if err := run(cfg, "ip", "netns", "add", nsName); err != nil {
		return "", fmt.Errorf("create network namespace: %w", err)
	}

	hostVeth, peerVeth := vethNames(cfg.ID)
	if err := run(cfg, "ip", "link", "add", hostVeth, "type", "veth", "peer", "name", peerVeth); err != nil {
		// Do not leave a namespace behind when a later step fails.
		_ = run(cfg, "ip", "netns", "delete", nsName)
		return "", fmt.Errorf("create veth pair: %w", err)
	}
	if err := run(cfg, "ip", "link", "set", peerVeth, "netns", nsName); err != nil {
		_ = run(cfg, "ip", "link", "del", hostVeth)
		_ = run(cfg, "ip", "netns", "delete", nsName)
		return "", fmt.Errorf("move veth into namespace: %w", err)
	}
	if err := run(cfg, "ip", "link", "set", hostVeth, "master", BridgeName); err != nil {
		return "", fmt.Errorf("attach veth to bridge: %w", err)
	}
	if err := run(cfg, "ip", "link", "set", hostVeth, "up"); err != nil {
		return "", fmt.Errorf("bring veth up: %w", err)
	}

	inNS := func(args ...string) error {
		return run(cfg, "ip", append([]string{"netns", "exec", nsName}, args...)...)
	}
	if err := inNS("ip", "link", "set", "lo", "up"); err != nil {
		return "", fmt.Errorf("bring loopback up: %w", err)
	}
	if err := inNS("ip", "addr", "add", alloc.Address+"/"+strconv.Itoa(prefixLen), "dev", peerVeth); err != nil {
		return "", fmt.Errorf("assign address: %w", err)
	}
	if err := inNS("ip", "link", "set", peerVeth, "up"); err != nil {
		return "", fmt.Errorf("bring container veth up: %w", err)
	}
	if err := inNS("ip", "route", "add", "default", "via", GatewayIP); err != nil {
		return "", fmt.Errorf("add default route: %w", err)
	}

	if err := EnableForwarding(cfg); err != nil {
		return "", err
	}
	if err := EnsureMasquerade(cfg); err != nil {
		return "", err
	}

	for _, pm := range cfg.Publish {
		if err := Publish(cfg, pm, alloc); err != nil {
			return "", err
		}
	}
	return netnsPath, nil
}

// Teardown releases everything Setup created. It is deliberately tolerant:
// a container that died badly may have already lost its namespace, and
// teardown must still finish so the address and rules are not leaked.
func Teardown(cfg Config) []error {
	var errs []error

	for _, pm := range cfg.Publish {
		if err := Unpublish(cfg, pm); err != nil {
			errs = append(errs, err)
		}
	}

	nsName := nsNameFor(cfg.ID)
	hostVeth, _ := vethNames(cfg.ID)
	if err := run(cfg, "ip", "link", "del", hostVeth); err != nil {
		// Absent is fine; the namespace delete below removes the peer.
		if !isMissing(err) {
			errs = append(errs, err)
		}
	}
	if err := run(cfg, "ip", "netns", "delete", nsName); err != nil && !isMissing(err) {
		errs = append(errs, err)
	}

	if err := Release(cfg.StateDir, cfg.ID); err != nil {
		errs = append(errs, err)
	}
	return errs
}

// requireRoot rejects rootless networking with a message that says why,
// rather than failing later with an opaque permission error.
func requireRoot() error {
	if os.Geteuid() == 0 {
		return nil
	}
	return fmt.Errorf(
		"container networking needs root (creating the bridge and NAT rules does); " +
			"the container will get loopback only — run with sudo for outbound networking and -p")
}

func requireTools() error {
	for _, tool := range []string{"ip", "iptables"} {
		if _, err := exec.LookPath(tool); err != nil {
			return fmt.Errorf("container networking needs %q on PATH: %w", tool, err)
		}
	}
	return nil
}

// run executes one host command, or records it under DryRun.
func run(cfg Config, name string, args ...string) error {
	if cfg.DryRun {
		if cfg.Out != nil {
			fmt.Fprintf(cfg.Out, "$ %s %s\n", name, strings.Join(args, " "))
		}
		return nil
	}
	cmd := exec.Command(name, args...)
	out, err := cmd.CombinedOutput()
	if err != nil {
		detail := strings.TrimSpace(string(out))
		if detail == "" {
			return fmt.Errorf("%s %s: %w", name, strings.Join(args, " "), err)
		}
		return fmt.Errorf("%s %s: %w: %s", name, strings.Join(args, " "), err, detail)
	}
	return nil
}

func isMissing(err error) bool {
	if err == nil {
		return false
	}
	msg := err.Error()
	return strings.Contains(msg, "Cannot find device") ||
		strings.Contains(msg, "No such file or directory") ||
		strings.Contains(msg, "does not exist")
}
