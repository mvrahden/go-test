//go:build linux

package proctree

import "syscall"

// deathSignal has the kernel kill the child when the thread that started it
// is gone: a runner that dies takes its trees with it.
func deathSignal(attr *syscall.SysProcAttr) { attr.Pdeathsig = syscall.SIGKILL }
