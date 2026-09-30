package security

// Seccomp filtering for Sentra containers.
//
// No libseccomp: that is a C library and Sentra is CGO_ENABLED=0. Instead
// this emits the seccomp profile in the OCI spec's JSON form, and runc
// compiles it to a BPF program at container start. The cost is a small
// allowlist and zero cgo.
//
// Policy shape, deliberately simple:
//
//	unconfined  no profile at all; an explicit escape hatch
//	default     a broad allowlist for ordinary workloads
//	strict      default minus the syscalls that reach into kernel, module
//	            or device internals, for images that do not need them
//
// The default list was assembled from syscall traces of real container
// workloads plus the syscalls Go, Node and Python runtimes are known to
// need. It is a starting point rather than a battle-tested set: when a
// workload is killed by a denied syscall, the fix is to add that syscall or
// run the container with --security unconfined.

import (
	"fmt"
	"sort"

	"github.com/opencontainers/runtime-spec/specs-go"
)

// Level selects how tightly a container is filtered.
type Level string

const (
	// LevelUnconfined installs no seccomp profile.
	LevelUnconfined Level = "unconfined"
	// LevelDefault is the broad allowlist used unless a Sentrafile says
	// otherwise.
	LevelDefault Level = "default"
	// LevelStrict tightens LevelDefault for images that need no access to
	// kernel, module or device internals.
	LevelStrict Level = "strict"
)

// Valid reports whether l is a level this package understands.
func (l Level) Valid() bool {
	switch l {
	case LevelUnconfined, LevelDefault, LevelStrict:
		return true
	}
	return false
}

const (
	actAllow = "SCMP_ACT_ALLOW"
	actErrno = "SCMP_ACT_ERRNO"
)

// defaultSyscalls is the allowlist for LevelDefault.
//
// Absent from this list, deliberately: mount, umount2, pivot_root, unshare,
// setns, open_tree, move_mount, mount_setattr, fsopen, fsconfig, fsmount,
// ptrace, process_vm_readv, process_vm_writev, bpf, perf_event_open,
// init_module, finit_module, delete_module, kexec_load, reboot, swapon,
// swapoff, open_by_handle_at, name_to_handle_at, and the io_uring family.
// Those either let a container escape its namespace or reach kernel
// internals, and no ordinary image needs them.
var defaultSyscalls = []string{
	"accept", "accept4", "access", "arch_prctl", "bind", "brk",
	"capget", "capset", "chdir", "chmod", "chown", "chown32", "clock_getres",
	"clock_gettime", "clock_nanosleep", "clone", "clone3", "close", "close_range",
	"connect", "copy_file_range", "creat", "dup", "dup2", "dup3",
	"epoll_create", "epoll_create1", "epoll_ctl", "epoll_pwait", "epoll_pwait2",
	"epoll_wait", "eventfd", "eventfd2", "execve", "execveat", "exit",
	"exit_group", "faccessat", "faccessat2", "fadvise64", "fadvise64_64",
	"fallocate", "fchdir", "fchmod", "fchmodat", "fchown", "fchown32", "fchownat",
	"fcntl", "fcntl64", "fdatasync", "fgetxattr", "flistxattr", "flock", "fork",
	"fremovexattr", "fsetxattr", "fstat", "fstat64", "fstatat64", "fstatfs",
	"fstatfs64", "fsync", "ftruncate", "ftruncate64", "futex",
	"futex_time64", "futex_waitv", "getcpu", "getcwd", "getdents",
	"getdents64", "getegid", "getegid32", "geteuid", "geteuid32", "getgid",
	"getgid32", "getgroups", "getgroups32", "getitimer", "getpeername",
	"getpgid", "getpgrp", "getpid", "getppid", "getpriority", "getrandom",
	"getresgid", "getresgid32", "getresuid", "getresuid32", "getrlimit",
	"get_robust_list", "getrusage", "getsid", "getsockname", "getsockopt",
	"gettid", "gettimeofday", "getuid", "getuid32", "getxattr",
	"inotify_add_watch", "inotify_init", "inotify_init1", "inotify_rm_watch",
	"ioctl", "ioprio_get", "ioprio_set", "kill", "lchown", "lchown32",
	"lgetxattr", "link", "linkat", "listen", "lremovexattr", "lseek",
	"lsetxattr", "lstat", "lstat64", "madvise", "membarrier", "memfd_create",
	"mkdir", "mkdirat", "mknod", "mlock", "mlock2", "mlockall", "mmap",
	"mmap2", "mprotect", "mremap", "msgctl", "msgget", "msgrcv", "msgsnd",
	"msync", "munlock", "munlockall", "munmap", "nanosleep", "newfstatat",
	"nice", "open", "openat", "openat2", "pause", "pipe", "pipe2", "poll",
	"ppoll", "ppoll_time64", "prctl", "pread64", "preadv", "preadv2",
	"prlimit64", "pselect6", "pselect6_time64", "pwrite64", "pwritev",
	"pwritev2", "read", "readahead", "readlink", "readlinkat", "readv",
	"recv", "recvfrom", "recvmmsg", "recvmsg", "removexattr", "rename",
	"renameat", "renameat2", "restart_syscall", "rmdir", "rseq", "rt_sigaction",
	"rt_sigpending", "rt_sigprocmask", "rt_sigqueueinfo", "rt_sigreturn",
	"rt_sigsuspend", "rt_sigtimedwait", "rt_tgsigqueueinfo",
	"sched_getaffinity", "sched_getattr", "sched_getparam",
	"sched_get_priority_max", "sched_get_priority_min", "sched_getscheduler",
	"sched_rr_get_interval", "sched_setaffinity", "sched_setattr",
	"sched_setparam", "sched_setscheduler", "sched_yield", "seccomp", "select",
	"send", "sendfile", "sendfile64", "sendmmsg", "sendmsg", "sendto",
	"setfsgid", "setfsgid32", "setfsuid", "setfsuid32", "setgid", "setgid32",
	"setgroups", "setgroups32", "setitimer", "setpgid", "setpriority",
	"setregid", "setregid32", "setresgid", "setresgid32", "setresuid",
	"setresuid32", "setreuid", "setreuid32", "setrlimit", "set_robust_list",
	"setsid", "setsockopt", "set_tid_address", "setuid", "setuid32", "setxattr",
	"shmat", "shmctl", "shmdt", "shmget", "shutdown", "sigaltstack", "signal",
	"signalfd4", "signalfd", "socket", "socketcall", "socketpair", "splice",
	"stat", "stat64", "statfs", "statfs64", "statx", "symlink", "symlinkat",
	"sync", "sync_file_range", "syncfs", "sysinfo", "tee", "tgkill", "time",
	"timer_create", "timer_delete", "timerfd_create", "timerfd_gettime",
	"timerfd_settime", "timer_getoverrun", "timer_gettime", "timer_settime",
	"times", "tkill", "truncate", "truncate64", "ugetrlimit", "umask",
	"uname", "unlink", "unlinkat", "utime", "utimensat", "utimensat_time64",
	"utimes", "vadvise", "vfork", "vmsplice", "wait4", "waitid", "waitpid",
	"write", "writev",
}

// strictSyscalls is LevelStrict: the subset a long-running service needs.
//
// The difference from LevelDefault is a statement about intent, not a
// rounding of the count. A service reads and writes its own files, talks to
// the network, manages threads, and handles signals. It does not install
// packages, compile, manage device nodes, poke at kernel accounting, or
// reach the kernel's keyring — so those are gone. Anything that needs them
// should declare it, and `--security unconfined` remains the escape hatch.
//
// The identity syscalls (setuid/setgid/setgroups and their relatives) are
// present in every level: the runtime needs them to establish a process's
// credentials during startup, and denying them kills the container before
// its entrypoint runs.
var strictSyscalls = []string{
	"accept", "accept4", "access", "arch_prctl", "bind", "brk",
	"chdir", "chmod", "chown", "clock_getres", "clock_gettime",
	"clock_nanosleep", "clone", "clone3", "close", "close_range", "connect",
	"copy_file_range", "creat", "dup", "dup2", "dup3", "epoll_create1",
	"epoll_ctl", "epoll_pwait", "epoll_pwait2", "epoll_wait", "eventfd2",
	"execve", "exit", "exit_group", "faccessat", "faccessat2", "fadvise64",
	"fadvise64_64", "fallocate", "fchdir", "fchmod", "fchmodat", "fchown",
	"fchownat", "fcntl", "fcntl64", "fdatasync", "flock", "fork", "fstat",
	"fstatat64", "fstatfs", "fsync", "ftruncate", "futex", "futex_time64",
	"futex_waitv", "getcpu", "getcwd", "getdents64", "getegid", "geteuid",
	"getgid", "getgroups", "getitimer", "getpeername", "getpid", "getppid",
	"getrandom", "getrlimit", "getsockname", "getsockopt", "gettid",
	"gettimeofday", "getuid", "ioctl",
	"kill", "lchown", "listen", "lseek", "lstat", "madvise", "membarrier",
	"memfd_create", "mkdir", "mkdirat", "mmap", "mprotect", "mremap", "msync",
	"munmap", "nanosleep", "newfstatat", "open", "openat", "openat2", "pause",
	"pipe", "pipe2", "poll", "ppoll", "prctl", "pread64", "preadv",
	"prlimit64", "pselect6", "pwrite64", "pwritev", "read",
	"readlink", "readlinkat", "readv", "recvfrom", "recvmmsg", "recvmsg",
	"renameat", "restart_syscall", "rmdir", "rseq", "rt_sigaction",
	"rt_sigpending", "rt_sigprocmask", "rt_sigreturn", "rt_sigtimedwait",
	"sched_getaffinity", "sched_yield", "seccomp", "select", "sendfile",
	"sendmmsg", "sendmsg", "sendto",
	"set_robust_list", "setrlimit", "setsid", "setsockopt", "set_tid_address",
	"setuid", "setuid32", "setgid", "setgid32", "setgroups", "setgroups32",
	"setfsuid", "setfsuid32", "setfsgid", "setfsgid32", "setreuid", "setreuid32",
	"setregid", "setregid32", "setresuid", "setresuid32", "setresgid",
	"setresgid32",
	"shutdown", "sigaltstack", "signal", "signalfd4", "socket", "socketpair",
	"splice", "stat", "statfs", "statx", "sync", "sync_file_range", "tee",
	"tgkill", "timer_create", "timer_delete", "timerfd_create",
	"timerfd_gettime", "timerfd_settime", "timer_gettime", "timer_settime",
	"times", "tkill", "truncate", "umask", "uname", "unlink", "unlinkat",
	"utimensat", "vfork", "wait4", "waitid", "write", "writev",
}

// Profile returns the OCI seccomp profile for a level. LevelUnconfined
// returns nil, which tells the runtime to install no filter.
func Profile(level Level) (*specs.LinuxSeccomp, error) {
	if !level.Valid() {
		return nil, fmt.Errorf("unknown security level %q (want default, strict or unconfined)", level)
	}
	if level == LevelUnconfined {
		return nil, nil
	}

	names := defaultSyscalls
	if level == LevelStrict {
		names = strictSyscalls
	}

	sorted := append([]string(nil), names...)
	sort.Strings(sorted)

	return &specs.LinuxSeccomp{
		DefaultAction: actErrno,
		Architectures: []specs.Arch{specs.ArchX86_64, specs.ArchX86, specs.ArchX32},
		Syscalls: []specs.LinuxSyscall{
			{
				Names:  sorted,
				Action: actAllow,
			},
		},
	}, nil
}

// SyscallCount reports how many syscalls a level allows, for reporting and
// tests.
func SyscallCount(level Level) (int, error) {
	profile, err := Profile(level)
	if err != nil || profile == nil {
		return 0, err
	}
	total := 0
	for _, call := range profile.Syscalls {
		total += len(call.Names)
	}
	return total, nil
}
