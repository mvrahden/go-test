//go:build dragonfly || freebsd || linux || netbsd || openbsd || solaris

package control

import "syscall"

// socketpair creates a connected pair, both ends close-on-exec from the
// start, so no process started meanwhile inherits either.
func socketpair() ([2]int, error) {
	return syscall.Socketpair(syscall.AF_UNIX, syscall.SOCK_STREAM|syscall.SOCK_CLOEXEC, 0)
}
