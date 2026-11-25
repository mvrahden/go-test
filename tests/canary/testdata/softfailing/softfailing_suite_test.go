// Package softfailing fails soft assertions: each failure is reported, the
// code after it runs, and the test fails at its end.
package softfailing

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

// SoftFailingTestSuite's methods each fail, and each runs past a failure
// only where the failure was soft.
type SoftFailingTestSuite struct{}

type row struct {
	Desc string
	ok   bool
}

func (s *SoftFailingTestSuite) TestTwoSoftFailures(t *gotest.T) {
	gotest.Equal(t.Soft(), "first soft", "failure")
	gotest.Equal(t.Soft(), "second soft", "failure")
}

func (s *SoftFailingTestSuite) TestAnItOnASoftT(t *gotest.T) {
	t.Soft().It("stops", func(it *gotest.T) {
		gotest.True(it, false)
		mark("after the failure in the It")
	})
}

func (s *SoftFailingTestSuite) TestEachRowFails(t *gotest.T) {
	for it, r := range gotest.Each(t, []row{{"first", true}, {"second", false}, {"third", true}}) {
		mark("row " + r.Desc)
		gotest.True(it, r.ok)
	}
}

func (s *SoftFailingTestSuite) TestEachRowStops(t *gotest.T) {
	for it, r := range gotest.Each(t, []row{{"first", true}, {"second", false}, {"third", true}}) {
		mark("stop " + r.Desc)
		if !r.ok {
			it.FailNow()
		}
	}
}
