package runtime

import "github.com/opencontainers/runtime-spec/specs-go"

// DefaultNamespaces returns the isolation set Sentra applies to every
// container: pid, net, mnt, uts, ipc.
//
// Root is required to create these namespaces directly. For rootless mode
// a user namespace is prepended (must be first in the list) so an
// unprivileged user can own the rest.
//
// When netnsPath is set, the network namespace is an existing one rather
// than a fresh, empty namespace. The network package creates the namespace
// before the container starts, because a veth has to be moved into it while
// nothing is using it, and runc needs to join the same one.
func DefaultNamespaces(rootless bool, netnsPath string) []specs.LinuxNamespace {
	ns := make([]specs.LinuxNamespace, 0, 6)
	if rootless {
		ns = append(ns, specs.LinuxNamespace{Type: specs.UserNamespace})
	}

	network := specs.LinuxNamespace{Type: specs.NetworkNamespace}
	if netnsPath != "" {
		network.Path = netnsPath
	}

	return append(ns,
		specs.LinuxNamespace{Type: specs.PIDNamespace},
		network,
		specs.LinuxNamespace{Type: specs.MountNamespace},
		specs.LinuxNamespace{Type: specs.UTSNamespace},
		specs.LinuxNamespace{Type: specs.IPCNamespace},
	)
}

// RootlessRequiresUserNamespace is why DefaultNamespaces prepends one: an
// unprivileged process cannot own the other namespaces directly. The uid/gid
// mapping that namespace requires lives in the security package.
