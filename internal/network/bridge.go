package network

// The bridge, its subnet, and address allocation.
//
// One bridge, one subnet, held in a small JSON file under the state dir.
// There is no DHCP server and no distributed store: an address is claimed by
// writing it down, and released by removing it. On a machine running a
// handful of containers that is the whole problem.

import (
	"encoding/json"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
)

const (
	// BridgeName is the single bridge every container attaches to.
	BridgeName = "sentra0"
	// GatewayIP is the bridge's own address, and the container's default
	// route.
	GatewayIP = "10.89.0.1"
	// subnet is the network containers are allocated from. 10.89.0.0/24 is
	// outside the ranges most home and office networks use, which keeps the
	// route from colliding with a real one.
	subnet = "10.89.0.0/24"
	// prefixLen is the CIDR length of subnet.
	prefixLen = 24
	// poolStart and poolEnd bound the addresses handed to containers. The
	// bridge holds .1.
	poolStart = 2
	poolEnd   = 254
	// netnsDir is where `ip netns` keeps its named namespaces.
	netnsDir = "/var/run/netns"
)

// Allocation is one container's claim on the subnet.
type Allocation struct {
	ID      string `json:"id"`
	Address string `json:"address"`
}

// allocationFile is the on-disk shape of the allocation table.
type allocationFile map[string]string

// Allocate claims an address for a container, reusing its previous claim
// when it has one so a restarted container keeps its address.
func Allocate(stateDir, id string) (Allocation, error) {
	table, err := loadAllocations(stateDir)
	if err != nil {
		return Allocation{}, err
	}
	if addr, ok := table[id]; ok {
		return Allocation{ID: id, Address: addr}, nil
	}

	used := make(map[string]bool, len(table))
	for _, addr := range table {
		used[addr] = true
	}
	prefix := strings.TrimSuffix(subnet, "/24")
	for host := poolStart; host <= poolEnd; host++ {
		candidate := fmt.Sprintf("%s.%d", prefix[:strings.LastIndex(prefix, ".")], host)
		if !used[candidate] {
			table[id] = candidate
			if err := saveAllocations(stateDir, table); err != nil {
				return Allocation{}, err
			}
			return Allocation{ID: id, Address: candidate}, nil
		}
	}
	return Allocation{}, fmt.Errorf("no free addresses in %s (%d in use)", subnet, len(used))
}

// Release forgets a container's claim. Releasing an unclaimed id is not an
// error: teardown must be able to run twice.
func Release(stateDir, id string) error {
	table, err := loadAllocations(stateDir)
	if err != nil {
		return err
	}
	if _, ok := table[id]; !ok {
		return nil
	}
	delete(table, id)
	return saveAllocations(stateDir, table)
}

// Allocations lists current claims, sorted by address, for `sentra network`.
func Allocations(stateDir string) ([]Allocation, error) {
	table, err := loadAllocations(stateDir)
	if err != nil {
		return nil, err
	}
	out := make([]Allocation, 0, len(table))
	for id, addr := range table {
		out = append(out, Allocation{ID: id, Address: addr})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Address < out[j].Address })
	return out, nil
}

func allocationsPath(stateDir string) string {
	return filepath.Join(stateDir, "network", "allocations.json")
}

func loadAllocations(stateDir string) (allocationFile, error) {
	table := allocationFile{}
	data, err := os.ReadFile(allocationsPath(stateDir))
	if err != nil {
		if os.IsNotExist(err) {
			return table, nil
		}
		return nil, err
	}
	if err := json.Unmarshal(data, &table); err != nil {
		return nil, fmt.Errorf("corrupt allocation table %s: %w", allocationsPath(stateDir), err)
	}
	return table, nil
}

func saveAllocations(stateDir string, table allocationFile) error {
	path := allocationsPath(stateDir)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	data, err := json.MarshalIndent(table, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0o644)
}

// EnsureBridge creates the bridge if it is missing and makes sure it has its
// gateway address and is up. Running it against an existing bridge is a
// no-op, which is what lets every container start call it.
func EnsureBridge(cfg Config) error {
	host, err := os.ReadFile("/sys/class/net/" + BridgeName + "/operstate")
	if err == nil && len(host) > 0 {
		return nil
	}

	if err := run(cfg, "ip", "link", "add", BridgeName, "type", "bridge"); err != nil {
		if !strings.Contains(err.Error(), "File exists") {
			return fmt.Errorf("create bridge %s: %w", BridgeName, err)
		}
	}
	if err := run(cfg, "ip", "addr", "add", GatewayIP+"/"+strconv.Itoa(prefixLen), "dev", BridgeName); err != nil {
		// An existing address is the normal case on the second container.
		if !strings.Contains(err.Error(), "File exists") {
			return fmt.Errorf("assign bridge address: %w", err)
		}
	}
	if err := run(cfg, "ip", "link", "set", BridgeName, "up"); err != nil {
		return fmt.Errorf("bring bridge up: %w", err)
	}
	return nil
}

// EnableForwarding turns on IPv4 forwarding, without which a container can
// reach the bridge but nothing beyond it.
func EnableForwarding(cfg Config) error {
	const path = "/proc/sys/net/ipv4/ip_forward"
	if cfg.DryRun {
		if cfg.Out != nil {
			fmt.Fprintf(cfg.Out, "$ write %s 1\n", path)
		}
		return nil
	}
	current, err := os.ReadFile(path)
	if err == nil && strings.TrimSpace(string(current)) == "1" {
		return nil
	}
	if err := os.WriteFile(path, []byte("1"), 0o644); err != nil {
		return fmt.Errorf("enable ipv4 forwarding (sysctl net.ipv4.ip_forward=1): %w", err)
	}
	return nil
}

// EnsureMasquerade makes container traffic look like it came from the host,
// so replies find their way back. The rule is idempotent.
func EnsureMasquerade(cfg Config) error {
	rule := []string{"-t", "nat", "-A", "POSTROUTING", "-s", subnet, "!", "-o", BridgeName, "-j", "MASQUERADE"}
	if err := checkRule(cfg, rule); err == nil {
		return nil
	}
	if err := run(cfg, "iptables", rule...); err != nil {
		return fmt.Errorf("add masquerade rule: %w", err)
	}
	return nil
}

// checkRule reports whether an iptables rule already exists.
func checkRule(cfg Config, rule []string) error {
	if cfg.DryRun {
		return fmt.Errorf("dry run")
	}
	// -C asks the kernel whether the rule is present without changing it.
	probe := append([]string{}, rule...)
	for i, a := range probe {
		if a == "-A" {
			probe[i] = "-C"
			break
		}
	}
	return run(cfg, "iptables", probe...)
}

// vethNames returns the host-side and container-side interface names for a
// container. Interface names are capped at 15 characters, so the id is
// truncated to fit.
func vethNames(id string) (host, peer string) {
	short := id
	if len(short) > 8 {
		short = short[:8]
	}
	return "sv" + short + "h", "sv" + short + "p"
}

// nsNameFor is the named-netns handle for a container.
func nsNameFor(id string) string {
	short := id
	if len(short) > 12 {
		short = short[:12]
	}
	return "sentra-" + short
}

// ParseCIDR is exported for tests that need the subnet's parts.
func ParseCIDR() (*net.IPNet, error) {
	_, block, err := net.ParseCIDR(subnet)
	return block, err
}
