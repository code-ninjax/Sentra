package security

// Rootless execution.
//
// A rootless container cannot own privileged namespaces directly, so it runs
// inside a user namespace it owns. That namespace needs an explicit uid/gid
// mapping, and for anything beyond a single id — an image that drops
// privileges, writes to a chowned volume, or runs a multi-user workload — it
// needs a *range*, taken from the invoking user's subordinate id allocations
// in /etc/subuid and /etc/subgid.
//
// A range matters for correctness, not just capability: with a one-id mapping
// an image whose entrypoint switches to uid 1000 has nowhere to switch to.

import (
	"bufio"
	"fmt"
	"os"
	"os/user"
	"strconv"
	"strings"

	"github.com/opencontainers/runtime-spec/specs-go"
)

// Mapping is a resolved subordinate id range for the invoking user.
type Mapping struct {
	HostID      uint32
	ContainerID uint32
	Size        uint32
	Source      string // "subuid", "subgid" or "single"
}

// Mappings resolves the uid and gid mappings for a rootless container.
//
// Sentra maps only the invoking user to container root, a single id rather
// than a full subordinate range. This is deliberate for now.
//
// A subuid range is the more correct design — it lets an image drop
// privileges to another uid — but it only works if every file in the image
// is owned by an id inside that range. Image layers are owned by host root
// (id 0), which is never inside a subordinate range, so runc tries to
// chown them at startup and fails with EPERM. Making a range usable means
// chowning every extracted layer file into the range at pull time, which
// Sentra does not do yet.
//
// A single-id mapping runs images that only read their filesystem, which is
// exactly Sentra's read-only default. It fails once a container needs to
// write to a root-owned path, and that limitation is real rather than
// hidden: it is the reason a writable rootless rootfs is not offered.
func Mappings() (uid, gid Mapping, err error) {
	uid = Mapping{HostID: uint32(os.Getuid()), ContainerID: 0, Size: 1, Source: "single"}
	gid = Mapping{HostID: uint32(os.Getgid()), ContainerID: 0, Size: 1, Source: "single"}
	return uid, gid, nil
}

// SubIDRangeFor reports the subordinate range allocated to the invoking
// user, if any. It exists so `sentra` can tell a user why rootless is
// running with a single id and what would unlock a full range.
func SubIDRangeFor() (Mapping, bool, error) {
	current, err := currentUser()
	if err != nil {
		return Mapping{}, false, err
	}
	m, ok := subIDRange("/etc/subuid", current)
	if !ok {
		return Mapping{}, false, nil
	}
	return Mapping{HostID: m.HostID, ContainerID: 0, Size: m.Size, Source: "subuid"}, true, nil
}

// currentUser returns the login name of the invoking user. getuid alone is
// not enough: /etc/subuid is keyed by name.
func currentUser() (string, error) {
	if u, err := user.Current(); err == nil && u.Username != "" {
		return u.Username, nil
	}
	if name := os.Getenv("USER"); name != "" {
		return name, nil
	}
	return "", fmt.Errorf("cannot determine the current user for subid lookup")
}

// subIDRange finds the first subordinate range allocated to a user. Files
// hold entries of the form "user:start:count".
func subIDRange(path, username string) (Mapping, bool) {
	f, err := os.Open(path)
	if err != nil {
		return Mapping{}, false
	}
	defer f.Close()

	sc := bufio.NewScanner(f)
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		parts := strings.Split(line, ":")
		if len(parts) != 3 || parts[0] != username {
			continue
		}
		start, err1 := strconv.ParseUint(parts[1], 10, 32)
		count, err2 := strconv.ParseUint(parts[2], 10, 32)
		if err1 != nil || err2 != nil || count == 0 {
			continue
		}
		return Mapping{HostID: uint32(start), Size: uint32(count)}, true
	}
	return Mapping{}, false
}

// ApplyMappings writes uid/gid mappings into a spec's Linux section.
func ApplyMappings(spec *specs.Spec, uid, gid Mapping) {
	if spec == nil || spec.Linux == nil {
		return
	}
	spec.Linux.UIDMappings = []specs.LinuxIDMapping{{
		ContainerID: uid.ContainerID,
		HostID:      uid.HostID,
		Size:        uid.Size,
	}}
	spec.Linux.GIDMappings = []specs.LinuxIDMapping{{
		ContainerID: gid.ContainerID,
		HostID:      gid.HostID,
		Size:        gid.Size,
	}}
}

// CheckRootless reports whether rootless containers can work here, and
// explains what to do if not. Failure is reported as an error rather than
// silently falling back to rootful execution.
func CheckRootless() error {
	if os.Geteuid() == 0 {
		return nil
	}
	probe, err := os.ReadFile("/proc/sys/kernel/unprivileged_userns_clone")
	if err != nil {
		return nil // not a sysctl on every kernel; assume allowed
	}
	if strings.TrimSpace(string(probe)) == "0" {
		return fmt.Errorf(
			"rootless containers are disabled on this kernel " +
				"(kernel.unprivileged_userns_clone=0); enable it or run as root")
	}
	return nil
}
