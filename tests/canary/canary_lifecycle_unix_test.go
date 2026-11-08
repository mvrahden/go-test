//go:build unix

package canary_test //nolint:fail-guard

import (
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/mvrahden/go-test/internal/testkit"
	"github.com/mvrahden/go-test/pkg/gotest"
)

// suiteSignallable: a single suite process can be signalled here.
const suiteSignallable = true

// signalSuite sends SIGTERM to the suite process whose pid its fixture marked
// as "pid=<n>", as a second stop signal reaches it.
func signalSuite(t *gotest.T, markersPath string) {
	got, _ := testkit.ReadMarkers(markersPath)
	for _, m := range got {
		if pid, ok := strings.CutPrefix(m, "pid="); ok {
			n, err := strconv.Atoi(pid)
			if err != nil {
				fatalf(t, "pid marker %q: %v", m, err)
			}
			if err := syscall.Kill(n, syscall.SIGTERM); err != nil {
				t.Errorf("signal the suite: %v", err)
			}
			return
		}
	}
	t.Errorf("no pid marker among %v", got)
}

// lateSignalWrapper is stopFileWrapper with the signal caught rather than
// ignored, so the suite keeps the signal's default action.
func lateSignalWrapper(t *gotest.T) ([]string, []string) {
	path := filepath.Join(t.TempDir(), "wrapper.sh")
	script := `#!/bin/sh
trap ':' INT TERM
pidfile="$GOTEST_STOP_FILE.pid"
"$@" &
child=$!
while [ "$(cat "$pidfile" 2>/dev/null)" != "$child" ]; do kill -0 "$child" 2>/dev/null || exit 1; sleep 0.02; done
echo $$ > "$pidfile"
echo "pid swapped" >> "$GOTEST_CANARY_DIR/markers"
status=0
while kill -0 "$child" 2>/dev/null; do wait "$child"; status=$?; done
exit $status
`
	if err := os.WriteFile(path, []byte(script), 0o700); err != nil { //nolint:gosec // G306: the script must be executable
		fatalf(t, "write wrapper: %v", err)
	}
	return []string{"-exec", path}, []string{"running", "pid swapped"}
}

// A signal that reaches the suite only after its stop file settled the run
// leaves it to end with its tests' own status: every verdict reaches the
// stream and the process lives to its own end.
func (s *CanaryLifecycleTestSuite) TestASignalAfterTheStopFileSettledTheRun(t *gotest.T) {
	wrap, held := lateSignalWrapper(t)
	r := s.startRun(t, t.TempDir(), []string{"STOPPING_LINGER=1"}, append(append([]string{"-json"}, wrap...), "./testdata/stopping/")...)
	if !r.await(90*time.Second, held...) {
		_, _ = r.end(0)
		fatalf(t, "latesignal: the run never held:\n%s%s", r.stdout.String(), r.stderr.String())
	}
	if err := testkit.Interrupt(r.cmd, r.tree); err != nil {
		_, _ = r.end(0)
		fatalf(t, "latesignal: signal: %v", err)
	}
	if !r.await(30*time.Second, "teardown done") {
		_, _ = r.end(0)
		fatalf(t, "latesignal: the run never settled:\n%s%s", r.stdout.String(), r.stderr.String())
	}
	signalSuite(t, r.markers)
	_, markers := r.end(15 * time.Second)
	out := r.stdout.String()
	if !slices.Contains(markers, "lingered") {
		t.Errorf("latesignal: markers %v, want lingered: the signal ended the process:\n%s", markers, r.stderr.String())
	}
	for _, path := range []string{"TestStopTestSuite/TestHolds", "TestStopTestSuite", ""} {
		if streamVerdict(out, path) == "" {
			t.Errorf("latesignal: no verdict for %q reached the stream:\n%s", path, out)
		}
	}
}
