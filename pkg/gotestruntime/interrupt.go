package gotestruntime

import (
	"flag"
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
// keeps Go's defaults. A fuzzing process keeps them too: Go stops fuzzing on an
// interrupt by itself, saving what it found, and m.Run returns to Main, which
// tears down then.
func watchStop() {
	watchOnce.Do(func() {
		if fuzzing() {
			return
		}
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

// fuzzing reports whether this process coordinates fuzzing or is one of its
// workers. Flags are parsed by the time a DAG registers: m.Run parses them.
func fuzzing() bool {
	if f := flag.Lookup("test.fuzz"); f != nil && f.Value.String() != "" {
		return true
	}
	f := flag.Lookup("test.fuzzworker")
	return f != nil && f.Value.String() == "true"
}

// stopping gives a second signal Go's default action back, so it ends the
// teardown at once, and keeps a closed output pipe from ending it first.
func stopping(sigs []os.Signal) {
	signal.Reset(sigs...)
	ignoreBrokenPipe()
}
