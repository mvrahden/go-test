// Package hold has an Exclusive suite and a suite of the e2e group; each
// stamps when it begins and ends, and under CROSSRUN_HOLD=1 holds until the
// test creates its run's release file.
package hold

import (
	"os"
	"path/filepath"
	"sync"
	"time"
)

var markMu sync.Mutex

// mark appends one line to the markers file the run under test reads back,
// prefixed by the run's CROSSRUN_ROLE.
func mark(event string) {
	markMu.Lock()
	defer markMu.Unlock()
	f, err := os.OpenFile(filepath.Join(os.Getenv("GOTEST_CANARY_DIR"), "markers"), os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o600)
	if err != nil {
		return
	}
	defer f.Close()
	_, _ = f.WriteString(os.Getenv("CROSSRUN_ROLE") + " " + event + "\n")
}

// span stamps name's begin, holds while the run is told to, and stamps its
// end. A hold the test never releases ends after a minute.
func span(name string) {
	mark(name + " begins")
	if os.Getenv("CROSSRUN_HOLD") == "1" {
		release := filepath.Join(os.Getenv("GOTEST_CANARY_DIR"), "release-"+os.Getenv("CROSSRUN_ROLE"))
		for deadline := time.Now().Add(time.Minute); time.Now().Before(deadline); time.Sleep(10 * time.Millisecond) {
			if _, err := os.Stat(release); err == nil {
				break
			}
		}
	}
	mark(name + " ends")
}
