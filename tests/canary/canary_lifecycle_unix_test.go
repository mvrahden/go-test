//go:build unix

package canary_test //nolint:fail-guard

import (
	"strconv"
	"strings"
	"syscall"

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
