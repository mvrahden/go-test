//go:build !linux

package gotestruntime

// No death signal to ask for here.
func askForParentDeath() {}
