package runtime

import "github.com/opencontainers/runtime-spec/specs-go"

// CgroupLimits holds the resource limits Sentra wires into cgroups v2 via
// the OCI runtime spec. runc translates these onto the v2 controllers;
// there is deliberately no cgroups v1 fallback.
//
// Rootless caveat: memory/cpu/pids delegation requires systemd user-slice
// delegation (systemd-run --user --scope -p MemoryMax=...) on the host.
// Without it, runc errors when applying resources for unprivileged users.
// We surface that error verbatim rather than silently dropping limits.
type CgroupLimits struct {
	MemoryLimitBytes int64
	CPUPeriod        uint64 // microseconds per period (default 100000)
	CPUQuota         int64  // microseconds of CPU per period; quota/period = cores
	PidsLimit        int64  // max processes, 0 = unlimited
}

func DefaultLimits() CgroupLimits { return CgroupLimits{} }

// Resources converts limits into the OCI spec's linux.resources block.
// Zero values are omitted so runc applies no constraint for them.
func (l CgroupLimits) Resources() *specs.LinuxResources {
	res := &specs.LinuxResources{}

	if l.MemoryLimitBytes > 0 {
		limit := l.MemoryLimitBytes
		res.Memory = &specs.LinuxMemory{Limit: &limit}
	}
	if l.CPUQuota > 0 || l.CPUPeriod > 0 {
		period := l.CPUPeriod
		if period == 0 {
			period = 100000
		}
		cpu := &specs.LinuxCPU{Period: &period}
		if l.CPUQuota > 0 {
			quota := l.CPUQuota
			cpu.Quota = &quota
		}
		res.CPU = cpu
	}
	if l.PidsLimit > 0 {
		res.Pids = &specs.LinuxPids{Limit: l.PidsLimit}
	}
	return res
}
