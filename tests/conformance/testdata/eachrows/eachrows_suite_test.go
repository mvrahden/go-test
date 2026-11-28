// Package eachrows runs Each tables in which one row fails, one row stops,
// one row skips, rows end the method's own T and one loop breaks, and rows
// that fail in a helper, an It, a T with a deadline and Eventually.
package eachrows

import (
	"os"
	"path/filepath"
	"time"

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

func (s *EachRowsTestSuite) TestAStoppingRow(t *gotest.T) {
	for it, r := range gotest.Each(t, []row{{"first", true}, {"second", false}, {"third", true}}) {
		mark("stop " + r.Desc)
		if !r.ok {
			it.FailNow()
		}
	}
	mark("after the stop")
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

func (s *EachRowsTestSuite) TestMethodSkipsInARow(t *gotest.T) {
	for _, r := range gotest.Each(t, []row{{"first", true}, {"second", true}}) {
		mark("method skip " + r.Desc)
		t.Skipf("the method is not for this run")
	}
}

func (s *EachRowsTestSuite) TestMethodFailsInARow(t *gotest.T) {
	for _, r := range gotest.Each(t, []row{{"first", true}, {"second", true}}) {
		mark("method fail " + r.Desc)
		gotest.Fail(t, "the method fails in a row")
	}
}

func (s *EachRowsTestSuite) TestAHelperInARow(t *gotest.T) {
	for it, r := range gotest.Each(t, []row{{"first", false}}) {
		expectOK(it, r.ok)
		mark("after the helper")
	}
}

func expectOK(t *gotest.T, ok bool) {
	gotest.True(t, ok, "the helper fails")
}

func (s *EachRowsTestSuite) TestAnItInARow(t *gotest.T) {
	for it, r := range gotest.Each(t, []row{{"first", false}}) {
		it.It("stops", func(it *gotest.T) {
			gotest.True(it, r.ok)
			mark("after the failure in the It")
		})
		mark("after the It")
	}
}

func (s *EachRowsTestSuite) TestADeadlineInARow(t *gotest.T) {
	for it, r := range gotest.Each(t, []row{{"first", false}}) {
		gotest.True(gotest.NewTWithDeadline(it, time.Minute), r.ok, "the deadline T fails")
		mark("after the deadline T")
	}
}

func (s *EachRowsTestSuite) TestEventuallyInARow(t *gotest.T) {
	for it, r := range gotest.Each(t, []row{{"first", false}}) {
		gotest.Eventually(it, 50*time.Millisecond, 10*time.Millisecond, func(poll *gotest.R) {
			gotest.True(poll, r.ok)
		})
		mark("after Eventually")
	}
}

// The methods below panic in a row that had already failed; each ends its
// own method, and the hooks and the method after them still run.

func (s *EachRowsTestSuite) AfterEach(t *gotest.T) { mark("after each " + t.Name()) }
func (s *EachRowsTestSuite) AfterAll(t *gotest.T)  { mark("after all") }

type valued struct {
	Desc string
	v    *int
}

func (s *EachRowsTestSuite) TestARowPanicsAfterFailing(t *gotest.T) {
	one := 1
	for it, r := range gotest.Each(t, []valued{{"first", &one}, {"second", nil}, {"third", &one}, {"fourth", &one}}) {
		mark("panic " + r.Desc)
		gotest.NotZero(it, r.v, "the %s row has no value", r.Desc)
		gotest.Equal(it, 1, *r.v)
	}
	mark("after the panic")
}

func (s *EachRowsTestSuite) TestPanicAfterAHelper(t *gotest.T) {
	for it, r := range gotest.Each(t, []row{{"first", false}}) {
		expectOK(it, r.ok)
		panic("after the helper")
	}
}

func (s *EachRowsTestSuite) TestPanicAfterADeadline(t *gotest.T) {
	for it, r := range gotest.Each(t, []row{{"first", false}}) {
		gotest.True(gotest.NewTWithDeadline(it, time.Minute), r.ok)
		panic("after the deadline T")
	}
}

func (s *EachRowsTestSuite) TestPanicAfterEventually(t *gotest.T) {
	for it, r := range gotest.Each(t, []row{{"first", false}}) {
		gotest.Eventually(it, 50*time.Millisecond, 10*time.Millisecond, func(poll *gotest.R) {
			gotest.True(poll, r.ok)
		})
		panic("after Eventually")
	}
}

func (s *EachRowsTestSuite) TestPanicAfterAnIt(t *gotest.T) {
	for it, r := range gotest.Each(t, []row{{"first", false}}) {
		it.It("stops", func(it *gotest.T) { gotest.True(it, r.ok) })
		panic("after the It")
	}
}

func (s *EachRowsTestSuite) TestTheMethodAfterThePanics(t *gotest.T) {
	mark("the method after the panics")
	gotest.True(t, true)
}
