package drill

import (
	"syscall"

	"golang.org/x/sys/unix"
)

func signalName(n int) string {
	if name := unix.SignalName(syscall.Signal(n)); name != "" {
		return name
	}
	return "signal " + itoa(n)
}
