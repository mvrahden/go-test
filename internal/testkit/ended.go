package testkit

import "time"

// EndedWithin reports whether the process pid has ended within d: it is gone,
// or it has exited and nobody has reaped it yet.
func EndedWithin(pid int, d time.Duration) bool {
	for deadline := time.Now().Add(d); ; time.Sleep(50 * time.Millisecond) {
		if !processAlive(pid) || zombie(pid) {
			return true
		}
		if time.Now().After(deadline) {
			return false
		}
	}
}
