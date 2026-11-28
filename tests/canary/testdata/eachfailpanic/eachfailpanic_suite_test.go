// Package eachfailpanic panics in an Each row whose assertion had failed:
// the row reports the panic, the method ends there and the suite goes on.
package eachfailpanic

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

type EachFailPanicTestSuite struct{}

type row struct {
	Desc string
	got  *int
}

func (s *EachFailPanicTestSuite) AfterEach(t *gotest.T) { mark("after each " + t.Name()) }
func (s *EachFailPanicTestSuite) AfterAll(t *gotest.T)  { mark("after all") }

func (s *EachFailPanicTestSuite) TestARowPanics(t *gotest.T) {
	one := 1
	for it, r := range gotest.Each(t, []row{{"first", &one}, {"second", nil}, {"third", &one}}) {
		mark("row " + r.Desc)
		gotest.NotZero(it, r.got, "the %s row has no value", r.Desc)
		gotest.Equal(it, 1, *r.got)
	}
	mark("after the loop")
}

func (s *EachFailPanicTestSuite) TestLater(t *gotest.T) {
	mark("later method")
	gotest.True(t, true)
}
