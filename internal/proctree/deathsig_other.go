//go:build unix && !linux

package proctree

import "syscall"

// deathSignal: only Linux ties a child's life to its parent's thread.
func deathSignal(*syscall.SysProcAttr) {}
