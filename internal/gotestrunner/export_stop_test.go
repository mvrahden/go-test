package gotestrunner

import (
	"os/exec"
	"time"

	"github.com/mvrahden/go-test/internal/control"
)

// ExportHelloWithin makes d the time a suite process has to take its
// channel, and returns the call that restores the default.
func ExportHelloWithin(d time.Duration) (restore func()) {
	prev := helloWithin
	helloWithin = d
	return func() { helloWithin = prev }
}

// ExportWithoutChannel starts suite processes without their end of the
// channel, as a wrapper that drops it would, until the returned call.
func ExportWithoutChannel() (restore func()) {
	attachChannel = func(*control.ChildEnd, *exec.Cmd) {}
	return func() { attachChannel = (*control.ChildEnd).Attach }
}
