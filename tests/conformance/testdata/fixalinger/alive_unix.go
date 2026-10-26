//go:build unix

package fixalinger

import "syscall"

// alive reports whether a process with pid exists.
func alive(pid int) bool { return syscall.Kill(pid, 0) == nil }
