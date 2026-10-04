package gotestruntime

import (
	"os"
	"os/signal"
	"sync"

	"github.com/mvrahden/go-test/internal/protocol"
)

var watchOnce sync.Once

// watchStop tears the registered fixtures down when the process is asked to
// stop, then ends it by the same signal. Go's default action for SIGINT and
// SIGTERM exits on the spot, so without it an interrupted run releases nothing.
//
// Under the gotest runner a signal counts only once the runner has created the
// stop file it names: a signal the code under test sends itself is left to the
// code under test. Under plain go test only SIGINT counts, the Ctrl-C a
// developer sends.
//
// It starts with the first DAG that came up; a process with nothing to release
// keeps Go's defaults.
func watchStop() {
	watchOnce.Do(func() {
		stopFile := os.Getenv(protocol.EnvStopFile)
		sigs := []os.Signal{os.Interrupt}
		if stopFile != "" {
			sigs = append(sigs, stopSignals...)
		}
		ch := make(chan os.Signal, 1)
		signal.Notify(ch, sigs...)
		go func() {
			for sig := range ch {
				if stopFile != "" {
					if _, err := os.Stat(stopFile); err != nil {
						continue
					}
				}
				stopping(sigs)
				runTeardowns()
				reraise(sig)
				return
			}
		}()
	})
}

// stopping gives a second signal Go's default action back, so it ends the
// teardown at once, and keeps a closed output pipe from ending it first.
func stopping(sigs []os.Signal) {
	signal.Reset(sigs...)
	ignoreBrokenPipe()
}
