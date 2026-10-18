//go:build unix

package gotestruntime

import "syscall"

func closeOnExec(fd int) { syscall.CloseOnExec(fd) }
