package gotestrunner

import (
	"bytes"
	"io"
	"strings"
	"sync"
)

// failureLogLimit bounds the reports a failureLog keeps.
const failureLogLimit = 20

// failureLog passes the shared fixture process's stderr on and keeps the
// lines in which the runtime reports a fixture's failure. Stderr is not part
// of the event stream, so without them a run's verdict can only say that
// some fixture failed.
type failureLog struct {
	out io.Writer

	mu      sync.Mutex
	partial []byte
	reports []string
}

func newFailureLog(out io.Writer) *failureLog {
	return &failureLog{out: out}
}

func (l *failureLog) Write(p []byte) (int, error) {
	l.mu.Lock()
	l.partial = append(l.partial, p...)
	for {
		line, rest, whole := bytes.Cut(l.partial, []byte("\n"))
		if !whole {
			break
		}
		l.partial = rest
		if report, ok := failureReport(string(line)); ok && len(l.reports) < failureLogLimit {
			l.reports = append(l.reports, report)
		}
	}
	l.mu.Unlock()
	return l.out.Write(p)
}

// take returns the reports kept since the last call.
func (l *failureLog) take() []string {
	l.mu.Lock()
	defer l.mu.Unlock()
	reports := l.reports
	l.reports = nil
	return reports
}

// failureReport recognizes the lines the runtime writes when a fixture's
// setup gave up or its teardown failed. Attempts that were retried are not
// failures of the fixture.
func failureReport(line string) (string, bool) {
	line = strings.TrimRight(line, "\r")
	if report, ok := strings.CutPrefix(line, "FAIL: "); ok {
		return report, true
	}
	if strings.Contains(line, ".AfterAll failed: ") || strings.Contains(line, ".AfterAll panicked: ") {
		return line, true
	}
	return "", false
}
