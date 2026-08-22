package runtime

import "github.com/opencontainers/runtime-spec/specs-go"

// DefaultNamespaces returns the isolation set Sentra applies to every
// container: pid, net, mnt, uts, ipc.
//
// Root is required to create these namespaces directly. For rootless mode
// a user namespace is prepended (must be first in the list) so an
// unprivileged user can own the rest. Network setup inside the netns —
// veth/bridge plumbing — lands in workstream 6; until then containers get
// loopback only.
func DefaultNamespaces(rootless bool) []specs.LinuxNamespace {
	ns := make([]specs.LinuxNamespace, 0, 6)
	if rootless {
		ns = append(ns, specs.LinuxNamespace{Type: specs.UserNamespace})
	}
	return append(ns,
		specs.LinuxNamespace{Type: specs.PIDNamespace},
		specs.LinuxNamespace{Type: specs.NetworkNamespace},
		specs.LinuxNamespace{Type: specs.MountNamespace},
		specs.LinuxNamespace{Type: specs.UTSNamespace},
		specs.LinuxNamespace{Type: specs.IPCNamespace},
	)
}
