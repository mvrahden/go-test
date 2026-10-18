//go:build linux

package proctree

import "syscall"

// deathSignal has the kernel send the child SIGTERM when the thread that
// started it is gone: a runner that dies asks its trees to stop, and a child
// that handles the request tears down before it exits. Only the root gets it;
// what the root started hears of it from the root.
func deathSignal(attr *syscall.SysProcAttr) { attr.Pdeathsig = syscall.SIGTERM }
