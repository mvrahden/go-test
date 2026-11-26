// Package eachfailing fails assertions in Each rows: each failure is
// reported, the row and the rows after it run, and the method fails.
package eachfailing

import (
	"os"
	"path/filepath"
	"time"

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

// EachFailingTestSuite's methods each fail in a row, and each runs past the
// failure only where an assertion on the row's T failed.
type EachFailingTestSuite struct{}

type row struct {
	Desc string
	ok   bool
}

func (s *EachFailingTestSuite) TestEachRowFails(t *gotest.T) {
	for it, r := range gotest.Each(t, []row{{"first", true}, {"second", false}, {"third", true}}) {
		mark("row " + r.Desc)
		gotest.True(it, r.ok, "first failure")
		gotest.True(it, r.ok, "second failure")
	}
}

func (s *EachFailingTestSuite) TestAnItInARow(t *gotest.T) {
	for it := range gotest.Each(t, []row{{"first", false}}) {
		it.It("stops", func(it *gotest.T) {
			gotest.True(it, false)
			mark("after the failure in the It")
		})
	}
}

func (s *EachFailingTestSuite) TestADeadlineInARow(t *gotest.T) {
	for it := range gotest.Each(t, []row{{"first", false}}) {
		gotest.True(gotest.NewTWithDeadline(it, time.Minute), false)
		mark("after the deadline T")
	}
}

func (s *EachFailingTestSuite) TestEachRowStops(t *gotest.T) {
	for it, r := range gotest.Each(t, []row{{"first", true}, {"second", false}, {"third", true}}) {
		mark("stop " + r.Desc)
		if !r.ok {
			it.FailNow()
		}
	}
}
