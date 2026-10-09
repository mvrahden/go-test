//go:build !unix && !windows

package gotestruntime

import "os"

// No stop signals beyond os.Interrupt, no SIGPIPE: js/wasm, wasip1, plan9.
var stopSignals []os.Signal

func notePipe() {}

func reraise(os.Signal) { os.Exit(1) }
