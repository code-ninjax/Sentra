package cli

import (
	"fmt"
	"os"

	"sentra/internal/network"
	"sentra/internal/runtime"
)

// cleanupNetwork releases a detached container's network resources: its
// published ports, veth, address and namespace.
//
// It is called from stop and rm rather than from run, because a detached
// container outlives the command that started it. Leaking a DNAT rule would
// leave a host port open to the network with nothing behind it, so this runs
// even when the container is already gone.
func cleanupNetwork(c *runtime.Container) {
	if c == nil || c.NetnsPath == "" {
		return
	}

	specs := c.Publish
	publish, err := network.ParsePortMaps(specs)
	if err != nil {
		fmt.Fprintf(os.Stderr, "sentra: network cleanup: %v\n", err)
		return
	}

	cfg := network.Config{
		ID:       c.ID,
		Publish:  publish,
		StateDir: stateRoot(),
		Out:      os.Stderr,
	}
	for _, err := range network.Teardown(cfg) {
		fmt.Fprintf(os.Stderr, "sentra: network cleanup: %v\n", err)
	}
}
