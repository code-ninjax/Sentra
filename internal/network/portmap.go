package network

// Published ports.
//
// A published port is two iptables rules: DNAT in the nat table to steer the
// connection to the container, and an ACCEPT in FORWARD so the kernel lets
// the redirected packet through. Both are added at container start and both
// are removed at teardown — a leaked DNAT rule is a port that stays open to
// the network after the container it belonged to is gone, which is the kind
// of bug that should be impossible to write here.
//
// The rules are addressed by the same match that created them, so removal
// does not depend on any stored state.

import (
	"fmt"
	"strconv"
	"strings"
)

// PortMap is one host-to-container port mapping.
type PortMap struct {
	HostPort      int
	ContainerPort int
	Protocol      string // "tcp" or "udp"
}

// ParsePortMap reads the `-p` syntax: "8080", "8080:80", "8080:80/udp".
//
// A single port means the same number on both sides. This is the one place
// that syntax is interpreted, so `sentra run -p` and a Sentrafile's
// `expose` cannot drift apart.
func ParsePortMap(spec string) (PortMap, error) {
	if spec == "" {
		return PortMap{}, fmt.Errorf("empty port mapping")
	}

	proto := "tcp"
	if idx := strings.Index(spec, "/"); idx >= 0 {
		proto = strings.ToLower(spec[idx+1:])
		spec = spec[:idx]
	}
	switch proto {
	case "tcp", "udp", "sctp":
	default:
		return PortMap{}, fmt.Errorf("unknown protocol %q in port mapping (want tcp or udp)", proto)
	}

	hostPart, containerPart := spec, ""
	hasColon := false
	if idx := strings.Index(spec, ":"); idx >= 0 {
		hostPart, containerPart = spec[:idx], spec[idx+1:]
		hasColon = true
	}

	hostPort, err := parsePort(hostPart)
	if err != nil {
		return PortMap{}, fmt.Errorf("host port %q: %w", hostPart, err)
	}

	containerPort := hostPort
	if hasColon {
		// "8080:" is a typo, not a request for port 8080.
		if containerPart == "" {
			return PortMap{}, fmt.Errorf("container port is missing after ':'")
		}
		containerPort, err = parsePort(containerPart)
		if err != nil {
			return PortMap{}, fmt.Errorf("container port %q: %w", containerPart, err)
		}
	}
	return PortMap{HostPort: hostPort, ContainerPort: containerPort, Protocol: proto}, nil
}

// ParsePortMaps reads several mappings, reporting which one was invalid.
func ParsePortMaps(specs []string) ([]PortMap, error) {
	out := make([]PortMap, 0, len(specs))
	for _, spec := range specs {
		pm, err := ParsePortMap(spec)
		if err != nil {
			return nil, fmt.Errorf("-p %s: %w", spec, err)
		}
		out = append(out, pm)
	}
	return out, nil
}

func parsePort(s string) (int, error) {
	n, err := strconv.Atoi(s)
	if err != nil {
		return 0, fmt.Errorf("not a number")
	}
	if n < 1 || n > 65535 {
		return 0, fmt.Errorf("out of range (1-65535)")
	}
	return n, nil
}

// String renders the mapping in the short form `sentra run -p` accepts, so
// container state can be read back into the same PortMap by ParsePortMap.
// Defaults are left out: tcp is implied, and a single port means both sides.
func (p PortMap) String() string {
	var b strings.Builder
	b.WriteString(strconv.Itoa(p.HostPort))
	if p.ContainerPort != p.HostPort {
		b.WriteString(":")
		b.WriteString(strconv.Itoa(p.ContainerPort))
	}
	if p.Protocol != "" && p.Protocol != "tcp" {
		b.WriteString("/")
		b.WriteString(p.Protocol)
	}
	return b.String()
}

// dnatRule is the nat-table rule steering a host port to the container.
func (p PortMap) dnatRule(containerIP string) []string {
	return []string{
		"-t", "nat", "-A", "PREROUTING",
		"-p", p.Protocol,
		"--dport", strconv.Itoa(p.HostPort),
		"-m", "comment", "--comment", "sentra:" + p.Protocol + ":" + strconv.Itoa(p.HostPort),
		"-j", "DNAT",
		"--to-destination", containerIP + ":" + strconv.Itoa(p.ContainerPort),
	}
}

// forwardRule lets the redirected packet through the FORWARD chain.
func (p PortMap) forwardRule(containerIP string) []string {
	return []string{
		"-A", "FORWARD",
		"-p", p.Protocol,
		"-d", containerIP,
		"--dport", strconv.Itoa(p.ContainerPort),
		"-j", "ACCEPT",
	}
}

// Publish opens a host port to a container.
func Publish(cfg Config, pm PortMap, alloc Allocation) error {
	for _, rule := range [][]string{pm.dnatRule(alloc.Address), pm.forwardRule(alloc.Address)} {
		if err := checkRule(cfg, rule); err == nil {
			continue // already published, perhaps by an earlier run
		}
		if err := run(cfg, "iptables", rule...); err != nil {
			return fmt.Errorf("publish %s: %w", pm, err)
		}
	}
	return nil
}

// Unpublish removes a container's port mapping. It uses the same rule
// matching as Publish, with -D instead of -A, so it removes exactly what was
// added and nothing else.
func Unpublish(cfg Config, pm PortMap) error {
	alloc, err := lookupAllocation(cfg.StateDir, cfg.ID)
	if err != nil || alloc.Address == "" {
		// Without the address the rules cannot be named. The container is
		// being removed regardless; report rather than guess.
		return fmt.Errorf("unpublish %s: container address unknown (rules may need manual cleanup)", pm)
	}

	var firstErr error
	for _, rule := range [][]string{pm.dnatRule(alloc.Address), pm.forwardRule(alloc.Address)} {
		del := append([]string{}, rule...)
		for i, a := range del {
			if a == "-A" {
				del[i] = "-D"
				break
			}
		}
		if err := run(cfg, "iptables", del...); err != nil && !isMissing(err) {
			if firstErr == nil {
				firstErr = err
			}
		}
	}
	return firstErr
}

func lookupAllocation(stateDir, id string) (Allocation, error) {
	table, err := loadAllocations(stateDir)
	if err != nil {
		return Allocation{}, err
	}
	addr, ok := table[id]
	if !ok {
		return Allocation{}, nil
	}
	return Allocation{ID: id, Address: addr}, nil
}
