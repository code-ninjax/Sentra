package security

import (
	"strings"
	"testing"

	"github.com/opencontainers/runtime-spec/specs-go"
)

func TestProfileLevels(t *testing.T) {
	for _, level := range []Level{LevelDefault, LevelStrict} {
		profile, err := Profile(level)
		if err != nil {
			t.Fatalf("Profile(%s): %v", level, err)
		}
		if profile == nil {
			t.Fatalf("Profile(%s) returned no profile", level)
		}
		if profile.DefaultAction != "SCMP_ACT_ERRNO" {
			t.Errorf("%s default action = %q, want SCMP_ACT_ERRNO (deny by default)",
				level, profile.DefaultAction)
		}
		if len(profile.Syscalls) != 1 {
			t.Fatalf("%s has %d syscall groups, want 1", level, len(profile.Syscalls))
		}
		if profile.Syscalls[0].Action != "SCMP_ACT_ALLOW" {
			t.Errorf("%s allow action = %q", level, profile.Syscalls[0].Action)
		}
		if len(profile.Syscalls[0].Names) < 50 {
			t.Errorf("%s allows only %d syscalls, too few to run a real image",
				level, len(profile.Syscalls[0].Names))
		}
	}
}

func TestProfileUnconfinedIsNil(t *testing.T) {
	profile, err := Profile(LevelUnconfined)
	if err != nil {
		t.Fatalf("Profile(unconfined): %v", err)
	}
	if profile != nil {
		t.Error("unconfined must install no seccomp profile")
	}
}

func TestProfileRejectsUnknownLevel(t *testing.T) {
	if _, err := Profile(Level("paranoid")); err == nil {
		t.Error("Profile accepted an unknown level")
	}
}

func TestStrictIsTighterThanDefault(t *testing.T) {
	def, err := SyscallCount(LevelDefault)
	if err != nil {
		t.Fatal(err)
	}
	strict, err := SyscallCount(LevelStrict)
	if err != nil {
		t.Fatal(err)
	}
	if strict >= def {
		t.Errorf("strict allows %d syscalls, default allows %d: strict must be tighter", strict, def)
	}
}

// Nothing in the allowlist may let a container change its own isolation.
// These are the syscalls an attacker reaches for first.
func TestEscapeSyscallsAreNeverAllowed(t *testing.T) {
	forbidden := []string{
		"mount", "umount2", "pivot_root", "chroot", "unshare", "setns",
		"open_tree", "move_mount", "mount_setattr", "fsopen", "fsconfig",
		"fsmount", "fspick", "open_by_handle_at", "name_to_handle_at",
		"ptrace", "process_vm_readv", "process_vm_writev", "bpf",
		"perf_event_open", "init_module", "finit_module", "delete_module",
		"kexec_load", "kexec_file_load", "reboot", "swapon", "swapoff",
		"iopl", "ioperm", "syslog", "acct",
	}

	for _, level := range []Level{LevelDefault, LevelStrict} {
		profile, err := Profile(level)
		if err != nil {
			t.Fatal(err)
		}
		allowed := map[string]bool{}
		for _, n := range profile.Syscalls[0].Names {
			allowed[n] = true
		}
		for _, bad := range forbidden {
			if allowed[bad] {
				t.Errorf("%s allows %s, which can break container isolation", level, bad)
			}
		}
	}
}

// The syscalls a normal process cannot run without. Losing any of these
// breaks ordinary images, which is the failure mode of an over-tight
// profile.
func TestEssentialSyscallsAreAllowed(t *testing.T) {
	essential := []string{
		"execve", "fork", "clone", "clone3", "exit", "exit_group", "wait4",
		"read", "write", "openat", "close", "fstat", "stat", "lstat", "statx",
		"mmap", "mprotect", "munmap", "mremap", "brk", "futex", "nanosleep",
		"rt_sigaction", "rt_sigprocmask", "rt_sigreturn", "sigaltstack",
		"arch_prctl", "set_tid_address", "set_robust_list", "rseq",
		"epoll_create1", "epoll_ctl", "epoll_wait", "eventfd2", "pipe2",
		"dup3", "fcntl", "ioctl", "getpid", "gettid", "getuid", "geteuid",
		"socket", "connect", "bind", "listen", "accept4", "sendto", "recvfrom",
		"shutdown", "setsockopt", "getsockopt", "uname", "getcwd", "chdir",
		"readlink", "readlinkat", "unlinkat", "renameat", "mkdirat", "chmod",
		"fchmod", "chown", "fchown", "lchown", "umask", "utimensat", "truncate",
		"getdents64", "madvise", "sched_yield", "sched_getaffinity",
		"membarrier", "getrandom", "prlimit64", "setrlimit", "getrlimit",
		"kill", "tgkill", "tkill", "openat2", "copy_file_range", "sendfile",
		"fsync", "fdatasync", "ftruncate", "flock", "signal", "pipe",
	}

	for _, level := range []Level{LevelDefault, LevelStrict} {
		profile, err := Profile(level)
		if err != nil {
			t.Fatal(err)
		}
		allowed := map[string]bool{}
		for _, n := range profile.Syscalls[0].Names {
			allowed[n] = true
		}
		for _, need := range essential {
			if !allowed[need] {
				t.Errorf("%s denies %s, which ordinary processes need", level, need)
			}
		}
	}
}

func TestProfileNamesAreSortedAndUnique(t *testing.T) {
	profile, err := Profile(LevelDefault)
	if err != nil {
		t.Fatal(err)
	}
	names := profile.Syscalls[0].Names
	seen := map[string]bool{}
	for i, n := range names {
		if seen[n] {
			t.Errorf("duplicate syscall %q", n)
		}
		seen[n] = true
		if i > 0 && names[i-1] > n {
			t.Errorf("syscall list is not sorted: %q before %q", names[i-1], n)
		}
		if strings.TrimSpace(n) != n || n == "" {
			t.Errorf("malformed syscall name %q", n)
		}
	}
}

func TestApplySetsPosture(t *testing.T) {
	spec := &specs.Spec{
		Root:  &specs.Root{Path: "/rootfs"},
		Linux: &specs.Linux{},
	}
	posture := Posture{Rootless: false, Readonly: true, Seccomp: LevelStrict}
	if err := Apply(spec, posture); err != nil {
		t.Fatalf("Apply: %v", err)
	}

	if !spec.Root.Readonly {
		t.Error("rootfs was not marked read-only")
	}
	if spec.Linux.Seccomp == nil {
		t.Fatal("no seccomp profile installed")
	}
	if len(spec.Linux.MaskedPaths) == 0 {
		t.Error("no masked paths on a read-only rootful posture")
	}
	if len(spec.Linux.ReadonlyPaths) == 0 {
		t.Error("no readonly paths on a read-only rootful posture")
	}
}

// Path masking is skipped rootless: runc fchowns the mountpoints it creates,
// which a user namespace cannot do, and the namespace already provides the
// protection the masking would.
func TestApplySkipsPathMaskingWhenRootless(t *testing.T) {
	spec := &specs.Spec{
		Root:  &specs.Root{Path: "/rootfs"},
		Linux: &specs.Linux{},
	}
	posture := Posture{Rootless: true, Readonly: true, Seccomp: LevelDefault}
	if err := Apply(spec, posture); err != nil {
		t.Fatalf("Apply: %v", err)
	}
	if !spec.Root.Readonly {
		t.Error("rootfs should still be read-only when rootless")
	}
	if len(spec.Linux.MaskedPaths) != 0 {
		t.Error("masked paths should not be requested rootless")
	}
	if len(spec.Linux.ReadonlyPaths) != 0 {
		t.Error("readonly paths should not be requested rootless")
	}
	if spec.Linux.Seccomp == nil {
		t.Error("seccomp must still apply when rootless")
	}
}

func TestApplyWritableDropsPathHardening(t *testing.T) {
	spec := &specs.Spec{
		Root:  &specs.Root{Path: "/rootfs"},
		Linux: &specs.Linux{},
	}
	posture := Posture{Rootless: true, Readonly: false, Seccomp: LevelDefault}
	if err := Apply(spec, posture); err != nil {
		t.Fatalf("Apply: %v", err)
	}
	if spec.Root.Readonly {
		t.Error("rootfs was marked read-only despite a writable posture")
	}
	if len(spec.Linux.MaskedPaths) != 0 {
		t.Error("masked paths should not apply to a writable rootfs")
	}
	if spec.Linux.Seccomp == nil {
		t.Error("seccomp should still apply to a writable rootfs")
	}
}

func TestApplyUnconfinedInstallsNoFilter(t *testing.T) {
	spec := &specs.Spec{
		Root:  &specs.Root{Path: "/rootfs"},
		Linux: &specs.Linux{},
	}
	if err := Apply(spec, Posture{Readonly: true, Seccomp: LevelUnconfined}); err != nil {
		t.Fatalf("Apply: %v", err)
	}
	if spec.Linux.Seccomp != nil {
		t.Error("unconfined posture installed a seccomp profile")
	}
}

func TestApplyRejectsSpecWithoutLinux(t *testing.T) {
	if err := Apply(&specs.Spec{}, Posture{}); err == nil {
		t.Error("Apply accepted a spec with no linux section")
	}
}

func TestDefaultPostureIsHardened(t *testing.T) {
	p := Default()
	if !p.Rootless {
		t.Error("default posture is not rootless")
	}
	if !p.Readonly {
		t.Error("default posture is not read-only")
	}
	if p.Seccomp != LevelDefault {
		t.Errorf("default seccomp level = %q", p.Seccomp)
	}
}

func TestSubIDRangeParsing(t *testing.T) {
	m, ok := subIDRange("testdata/subuid", "builder")
	if !ok {
		t.Fatal("expected a range for the builder user")
	}
	if m.HostID != 100000 || m.Size != 65536 {
		t.Errorf("range = host %d size %d, want host 100000 size 65536", m.HostID, m.Size)
	}

	if _, ok := subIDRange("testdata/subuid", "nobody"); ok {
		t.Error("found a range for a user that has none")
	}
	if _, ok := subIDRange("testdata/missing", "builder"); ok {
		t.Error("found a range in a file that does not exist")
	}
}
