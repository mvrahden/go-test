//go:build !linux

package gotestruntime

import "os"

// No death signal comes here: the watcher's poll sees the parent go.
var deathSignal os.Signal
