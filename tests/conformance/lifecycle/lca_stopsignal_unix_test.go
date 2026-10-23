//go:build unix

package lifecycle_test

import (
	"strconv"
	"strings"
	"syscall"

	"github.com/mvrahden/go-test/internal/testkit"
)

// suiteSignallable: a single suite process can be signalled here.
const suiteSignallable = true

// signalSuite sends SIGTERM to the suite process whose pid its fixture marked
// as "pid=<n>", as a second stop signal reaches it.
func signalSuite(markers string) error {
	got, _ := testkit.ReadMarkers(markers)
	for _, m := range got {
		if pid, ok := strings.CutPrefix(m, "pid="); ok {
			n, err := strconv.Atoi(pid)
			if err != nil {
				return err
			}
			return syscall.Kill(n, syscall.SIGTERM)
		}
	}
	return syscall.ESRCH
}
