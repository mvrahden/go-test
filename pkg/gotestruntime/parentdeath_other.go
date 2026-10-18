//go:build !linux && !darwin && !dragonfly && !freebsd && !netbsd && !openbsd && !windows

package gotestruntime

// No death signal to ask for here.
func askForParentDeath() {}

// No way to see the parent end here but its pid.
func parentExited() bool { return false }
