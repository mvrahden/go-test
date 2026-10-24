//go:build unix

package testkit

import (
	"errors"
	"os"
	"strconv"
	"strings"
	"syscall"
)

// processAlive reports whether a process with pid is running.
func processAlive(pid int) bool {
	// 0 and -1 would signal a group, not a process.
	if pid <= 0 {
		return false
	}
	proc, err := os.FindProcess(pid)
	if err != nil {
		return false
	}
	err = proc.Signal(syscall.Signal(0))
	return err == nil || errors.Is(err, syscall.EPERM)
}

// zombie reports whether pid has exited and nobody has reaped it yet, as an
// orphan waits for a PID 1 that reaps late. Only Linux can say.
func zombie(pid int) bool {
	stat, err := os.ReadFile("/proc/" + strconv.Itoa(pid) + "/stat")
	if err != nil {
		return false
	}
	s := string(stat)
	return strings.Contains(s[strings.LastIndexByte(s, ')'):], ") Z ")
}
