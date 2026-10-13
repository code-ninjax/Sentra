//go:build windows

package main

import "testing"

func TestToWSLPath(t *testing.T) {
	cases := []struct{ in, want string }{
		{`C:\Users\hp\rootfs`, "/mnt/c/Users/hp/rootfs"},
		{`C:/Users/hp/rootfs`, "/mnt/c/Users/hp/rootfs"},
		{`D:\data`, "/mnt/d/data"},
		{`.\rootfs`, "./rootfs"},
		{`--name`, "--name"},
		{`alpine:3.20`, "alpine:3.20"},
		{`--memory=512`, "--memory=512"},
		{`/bin/echo`, "/bin/echo"},
		{`--`, "--"},
		{`Z:\a\b\c`, "/mnt/z/a/b/c"},
	}
	for _, c := range cases {
		if got := toWSLPath(c.in); got != c.want {
			t.Errorf("toWSLPath(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}
