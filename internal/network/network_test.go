package network

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestParsePortMap(t *testing.T) {
	tests := []struct {
		spec string
		want PortMap
	}{
		{"8080", PortMap{HostPort: 8080, ContainerPort: 8080, Protocol: "tcp"}},
		{"8080:80", PortMap{HostPort: 8080, ContainerPort: 80, Protocol: "tcp"}},
		{"8080:80/udp", PortMap{HostPort: 8080, ContainerPort: 80, Protocol: "udp"}},
		{"53:53/UDP", PortMap{HostPort: 53, ContainerPort: 53, Protocol: "udp"}},
		{"1", PortMap{HostPort: 1, ContainerPort: 1, Protocol: "tcp"}},
		{"65535:65535", PortMap{HostPort: 65535, ContainerPort: 65535, Protocol: "tcp"}},
	}
	for _, tt := range tests {
		got, err := ParsePortMap(tt.spec)
		if err != nil {
			t.Errorf("ParsePortMap(%q): %v", tt.spec, err)
			continue
		}
		if got != tt.want {
			t.Errorf("ParsePortMap(%q) = %+v, want %+v", tt.spec, got, tt.want)
		}
	}
}

func TestParsePortMapRejects(t *testing.T) {
	bad := []string{
		"", "0", "65536", "http", "8080:", ":80", "8080:0",
		"8080/quic", "8080:", "abc:80", "8080:abc",
	}
	for _, spec := range bad {
		if _, err := ParsePortMap(spec); err == nil {
			t.Errorf("ParsePortMap(%q) succeeded, want an error", spec)
		}
	}
}

func TestPortMapStringRoundTrips(t *testing.T) {
	// Canonical form: tcp is implied and a single port means both sides.
	for _, spec := range []string{"8080", "8080:80", "8080:80/udp", "53/udp"} {
		pm, err := ParsePortMap(spec)
		if err != nil {
			t.Fatalf("ParsePortMap(%q): %v", spec, err)
		}
		if got := pm.String(); got != spec {
			t.Errorf("String() = %q, want %q", got, spec)
		}
	}
}

// State stores the short form, so re-parsing it must yield the same mapping
// even when the original was written with redundant detail.
func TestPortMapReParsesFromState(t *testing.T) {
	written := []string{"8080:80", "53:53/udp", "3000/tcp"}
	parsed, err := ParsePortMaps(written)
	if err != nil {
		t.Fatal(err)
	}
	for i, pm := range parsed {
		again, err := ParsePortMap(pm.String())
		if err != nil {
			t.Fatalf("re-parse %q: %v", pm.String(), err)
		}
		if again != pm {
			t.Errorf("round trip changed %+v into %+v", pm, again)
		}
		if again.String() != pm.String() {
			t.Errorf("re-render is unstable: %q then %q", pm.String(), again.String())
		}
		_ = i
	}
}

func TestDNATRuleTargetsTheContainer(t *testing.T) {
	pm := PortMap{HostPort: 8080, ContainerPort: 80, Protocol: "tcp"}
	rule := strings.Join(pm.dnatRule("10.89.0.7"), " ")

	for _, want := range []string{
		"-t nat", "-A PREROUTING", "-p tcp", "--dport 8080",
		"-j DNAT", "--to-destination 10.89.0.7:80",
	} {
		if !strings.Contains(rule, want) {
			t.Errorf("dnat rule missing %q:\n%s", want, rule)
		}
	}
	if strings.Contains(rule, "MASQUERADE") {
		t.Errorf("dnat rule should not masquerade:\n%s", rule)
	}
}

func TestForwardRuleAllowsOnlyTheContainer(t *testing.T) {
	pm := PortMap{HostPort: 8080, ContainerPort: 80, Protocol: "tcp"}
	rule := strings.Join(pm.forwardRule("10.89.0.7"), " ")

	if !strings.Contains(rule, "-d 10.89.0.7") {
		t.Errorf("forward rule is not scoped to the container:\n%s", rule)
	}
	if !strings.Contains(rule, "-j ACCEPT") {
		t.Errorf("forward rule does not accept:\n%s", rule)
	}
	if strings.Contains(rule, "-s ") {
		t.Errorf("forward rule should not match on source:\n%s", rule)
	}
}

// Removal must name exactly what creation added, or a stopped container
// leaves an open port behind.
func TestUnpublishRuleMatchesPublishRule(t *testing.T) {
	stateDir := t.TempDir()
	if _, err := Allocate(stateDir, "abc"); err != nil {
		t.Fatal(err)
	}

	pm := PortMap{HostPort: 8080, ContainerPort: 80, Protocol: "tcp"}
	cfg := Config{ID: "abc", StateDir: stateDir, Out: &bytes.Buffer{}, DryRun: true}

	// Record what Publish would run.
	var created bytes.Buffer
	cfg.Out = &created
	if err := Publish(cfg, pm, Allocation{ID: "abc", Address: "10.89.0.2"}); err != nil {
		t.Fatalf("Publish: %v", err)
	}

	var removed bytes.Buffer
	cfg.Out = &removed
	if err := Unpublish(cfg, pm); err != nil {
		t.Fatalf("Unpublish: %v", err)
	}

	createdCmds := normalize(strings.Split(strings.TrimSpace(created.String()), "\n"))
	removedCmds := normalize(strings.Split(strings.TrimSpace(removed.String()), "\n"))

	if len(createdCmds) != len(removedCmds) {
		t.Fatalf("publish ran %d commands, unpublish ran %d:\n%s\n---\n%s",
			len(createdCmds), len(removedCmds), created.String(), removed.String())
	}
	for i := range createdCmds {
		added := strings.Replace(createdCmds[i], "-A", "-D", 1)
		removed := strings.Replace(removedCmds[i], "-C", "-D", 1)
		if added != removed {
			t.Errorf("rule %d does not match:\n  created: %s\n  removed: %s", i, added, removed)
		}
	}
}

func normLine(s string) string { return strings.Join(strings.Fields(s), " ") }

func normalize(lines []string) []string {
	out := make([]string, 0, len(lines))
	for _, l := range lines {
		l = strings.TrimSpace(l)
		if l == "" {
			continue
		}
		out = append(out, normLine(strings.TrimPrefix(l, "$ ")))
	}
	return out
}

func TestEnsureMasqueradeIsIdempotent(t *testing.T) {
	cfg := Config{ID: "abc", StateDir: t.TempDir(), Out: &bytes.Buffer{}, DryRun: true}
	if err := EnsureMasquerade(cfg); err != nil {
		t.Fatalf("EnsureMasquerade: %v", err)
	}
	got := cfg.Out.(*bytes.Buffer).String()
	if !strings.Contains(got, "MASQUERADE") {
		t.Errorf("expected a masquerade rule, got:\n%s", got)
	}
	if !strings.Contains(got, subnet) {
		t.Errorf("masquerade should be scoped to %s, got:\n%s", subnet, got)
	}
	if !strings.Contains(got, "! -o "+BridgeName) {
		t.Errorf("masquerade should not apply to the bridge itself, got:\n%s", got)
	}
}

func TestAllocateIsStableAndUnique(t *testing.T) {
	stateDir := t.TempDir()

	first, err := Allocate(stateDir, "one")
	if err != nil {
		t.Fatal(err)
	}
	second, err := Allocate(stateDir, "two")
	if err != nil {
		t.Fatal(err)
	}
	if first.Address == second.Address {
		t.Fatalf("two containers got the same address %s", first.Address)
	}
	if first.Address != "10.89.0.2" {
		t.Errorf("first address = %s, want 10.89.0.2", first.Address)
	}
	if second.Address != "10.89.0.3" {
		t.Errorf("second address = %s, want 10.89.0.3", second.Address)
	}

	// Reallocating the same id keeps its address, so a restarted container
	// does not silently move.
	again, err := Allocate(stateDir, "one")
	if err != nil {
		t.Fatal(err)
	}
	if again.Address != first.Address {
		t.Errorf("reallocate moved %s to %s", first.Address, again.Address)
	}
}

func TestReleaseFreesAddressForReuse(t *testing.T) {
	stateDir := t.TempDir()
	first, err := Allocate(stateDir, "one")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := Allocate(stateDir, "two"); err != nil {
		t.Fatal(err)
	}

	if err := Release(stateDir, "one"); err != nil {
		t.Fatal(err)
	}
	third, err := Allocate(stateDir, "three")
	if err != nil {
		t.Fatal(err)
	}
	if third.Address != first.Address {
		t.Errorf("released address %s was not reused, got %s", first.Address, third.Address)
	}

	// Releasing twice is not an error: teardown must be safe to repeat.
	if err := Release(stateDir, "one"); err != nil {
		t.Errorf("second release failed: %v", err)
	}
}

func TestAllocateExhaustion(t *testing.T) {
	stateDir := t.TempDir()
	for i := poolStart; i <= poolEnd; i++ {
		id := "c" + string(rune('a'+i%26)) + string(rune('a'+i/26))
		if _, err := Allocate(stateDir, id); err != nil {
			t.Fatalf("allocate %d: %v", i, err)
		}
	}
	if _, err := Allocate(stateDir, "overflow"); err == nil {
		t.Error("allocation succeeded with the pool exhausted")
	}
}

func TestAllocationsSurviveReload(t *testing.T) {
	stateDir := t.TempDir()
	if _, err := Allocate(stateDir, "one"); err != nil {
		t.Fatal(err)
	}
	got, err := Allocations(stateDir)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || got[0].ID != "one" {
		t.Fatalf("Allocations = %+v, want one entry for one", got)
	}
	if _, err := os.Stat(filepath.Join(stateDir, "network", "allocations.json")); err != nil {
		t.Errorf("allocation table was not persisted: %v", err)
	}
}

func TestCorruptAllocationTableIsReported(t *testing.T) {
	stateDir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(stateDir, "network"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(allocationsPath(stateDir), []byte("not json"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := Allocate(stateDir, "one"); err == nil {
		t.Error("a corrupt allocation table was silently ignored")
	}
}

func TestVethNamesFitTheInterfaceLimit(t *testing.T) {
	host, peer := vethNames("0123456789abcdef0123456789abcdef")
	if len(host) > 15 || len(peer) > 15 {
		t.Errorf("interface names exceed 15 chars: %q %q", host, peer)
	}
	if host == peer {
		t.Errorf("host and peer names are identical: %q", host)
	}
}

// Namespace names become filenames under /var/run/netns, so they need to be
// legitimate path components rather than bounded by the interface-name limit.
func TestNamespaceNameIsAPathComponent(t *testing.T) {
	name := nsNameFor(strings.Repeat("x", 64))
	if name == "" || strings.ContainsAny(name, "/\x00") {
		t.Fatalf("namespace name %q is not a usable path component", name)
	}
	if !strings.HasPrefix(name, "sentra-") {
		t.Errorf("namespace name %q lost its prefix", name)
	}
	if len(name) > 64 {
		t.Errorf("namespace name %q is unreasonably long", name)
	}
}

func TestSetupRefusesRootless(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("running as root; the rootless guard does not apply")
	}
	_, err := Setup(Config{ID: "abc", StateDir: t.TempDir(), Out: &bytes.Buffer{}})
	if err == nil {
		t.Fatal("Setup succeeded without root")
	}
	if !strings.Contains(err.Error(), "needs root") {
		t.Errorf("error should explain the root requirement, got: %v", err)
	}
}
