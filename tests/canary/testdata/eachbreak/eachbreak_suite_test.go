// Package eachbreak breaks out of an Each loop after its first row.
package eachbreak

import (
	"os"
	"path/filepath"

	"github.com/mvrahden/go-test/pkg/gotest"
)

// mark appends one line to the markers file the canary reads back.
func mark(event string) {
	f, err := os.OpenFile(filepath.Join(os.Getenv("GOTEST_CANARY_DIR"), "markers"), os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o600)
	if err != nil {
		return
	}
	defer f.Close()
	_, _ = f.WriteString(event + "\n")
}

// EachBreakTestSuite's method must go on after the loop it broke.
type EachBreakTestSuite struct{}

type row struct{ Desc string }

func (s *EachBreakTestSuite) TestBreaksAfterTheFirstRow(t *gotest.T) {
	for _, r := range gotest.Each(t, []row{{"first"}, {"second"}, {"third"}}) {
		mark("row " + r.Desc)
		break
	}
	mark("after the loop")
}
