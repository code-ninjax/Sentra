//go:build windows

// Sentra on Windows runs through WSL2: this binary is a thin forwarder,
// not the runtime. It translates Windows paths in args (C:\x -> /mnt/c/x)
// and execs `wsl.exe -- sentra <args>`, passing stdio and exit codes
// straight through. All container work (namespaces, cgroups, overlayfs)
// happens inside the WSL2 distro where the real sentra binary lives.
//
// Stdlib only: forwarder must stay tiny (~2MB) next to the real binary.
package main

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

const hint = `sentra — daemonless OCI container runtime (Windows entrypoint)

This binary forwards to sentra inside WSL2:
  wsl.exe -- sentra <args>

Requires: WSL2 with a distro that has the sentra Linux binary installed,
e.g.  wsl -d Ubuntu -- sudo install -m755 sentra-linux /usr/local/bin/sentra
`

// toWSLPath converts a Windows path to its /mnt/<drive>/... equivalent.
// Non-path args are returned unchanged. A path is only translated when it
// looks like Windows (drive letter or UNC/backslashes) to avoid mangling
// container-side args like --name or image refs.
func toWSLPath(arg string) string {
	// Drive-letter form: C:\x, C:/x, or bare C: (rare, leave bare alone).
	if len(arg) >= 3 && arg[1] == ':' && (arg[2] == '\\' || arg[2] == '/') {
		drive := strings.ToLower(string(arg[0]))
		rest := filepath.ToSlash(arg[3:])
		return "/mnt/" + drive + "/" + rest
	}
	// Any other arg containing a backslash is almost certainly a Windows
	// path (.\rootfs, ..\data). Translate separators and, if absolute
	// (\\server\share), map to /mnt-relative form.
	if strings.ContainsRune(arg, '\\') {
		p := filepath.ToSlash(arg)
		p = strings.TrimPrefix(p, "//")
		return p
	}
	return arg
}

func main() {
	if len(os.Args) < 2 {
		fmt.Fprint(os.Stderr, hint)
		os.Exit(2)
	}

	wsl, err := exec.LookPath("wsl.exe")
	if err != nil {
		fmt.Fprintln(os.Stderr, "sentra: wsl.exe not found — install WSL2 first (wsl --install)")
		os.Exit(1)
	}

	fwd := make([]string, 0, len(os.Args)+1)
	fwd = append(fwd, "--", "sentra")
	for _, a := range os.Args[1:] {
		fwd = append(fwd, toWSLPath(a))
	}

	cmd := exec.Command(wsl, fwd...)
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		if ee, ok := err.(*exec.ExitError); ok {
			os.Exit(ee.ExitCode())
		}
		fmt.Fprintln(os.Stderr, "sentra:", err)
		os.Exit(1)
	}
}
