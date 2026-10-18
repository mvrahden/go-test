// Package eachrows runs Each tables in which one row fails, one row skips
// and one loop breaks.
package eachrows

import (
	"os"
	"path/filepath"

	"github.com/mvrahden/go-test/pkg/gotest"
)

// mark appends one line to the markers file the run under test reads back.
func mark(event string) {
	f, err := os.OpenFile(filepath.Join(os.Getenv("GOTEST_CANARY_DIR"), "markers"), os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o600)
	if err != nil {
		return
	}
	defer f.Close()
	_, _ = f.WriteString(event + "\n")
}

type EachRowsTestSuite struct{}

type row struct {
	Desc string
	ok   bool
}

func (s *EachRowsTestSuite) TestAFailingRow(t *gotest.T) {
	for it, r := range gotest.Each(t, []row{{"first", true}, {"second", false}, {"third", true}}) {
		mark("row " + r.Desc)
		gotest.True(it, r.ok, "the %s row fails", r.Desc)
	}
	mark("after the loop")
}

func (s *EachRowsTestSuite) TestBreak(t *gotest.T) {
	for it, r := range gotest.Each(t, []row{{"first", true}, {"second", true}, {"third", true}}) {
		mark("break " + r.Desc)
		gotest.True(it, r.ok)
		break
	}
	mark("after the break")
}

func (s *EachRowsTestSuite) TestASkippingRow(t *gotest.T) {
	for it, r := range gotest.Each(t, []row{{"first", true}, {"second", false}, {"third", true}}) {
		mark("skip " + r.Desc)
		if !r.ok {
			it.Skipf("the %s row is not for this run", r.Desc)
		}
	}
	mark("after the skip")
}
