// Ring 0: raw checks only (see ring0_suite_test.go).
package gotestruntime_test //nolint:fail-guard

import (
	"bytes"
	"strings"
	"testing"
	"testing/synctest"
	"time"

	"github.com/mvrahden/go-test/internal/runstate"
	"github.com/mvrahden/go-test/pkg/gotest"
	"github.com/mvrahden/go-test/pkg/gotestruntime"
)

// DefaultDeadlineTestSuite covers the line that says a context ended at the
// default deadline of a suite that declares none.
// Sequential: it redirects the line and stops the run.
type DefaultDeadlineTestSuite struct {
	out     *bytes.Buffer
	restore func()
}

func (s *DefaultDeadlineTestSuite) BeforeEach(t *gotest.T) {
	s.out = &bytes.Buffer{}
	s.restore = gotestruntime.ExportDefaultDeadlineOut(s.out)
}

func (s *DefaultDeadlineTestSuite) AfterEach(t *gotest.T) {
	s.restore()
	gotestruntime.ExportResetStop()
}

// inBubble runs phase on a test of its own in a synctest bubble and returns
// that test's name and the lines written about the default deadline.
func (s *DefaultDeadlineTestSuite) inBubble(t *gotest.T, phase func(st *testing.T)) (name string, lines []string) {
	synctest.Test(t.T(), func(st *testing.T) {
		name = st.Name()
		phase(st)
		// Long past every deadline: a line still to come would be written now.
		time.Sleep(time.Minute)
	})
	return name, strings.Split(strings.TrimSpace(s.out.String()), "\n")
}

func waitOn(tt *gotest.T) { <-tt.Context().Done() }

func (s *DefaultDeadlineTestSuite) TestAMethodPastTheDefaultIsNamed(t *gotest.T) {
	name, lines := s.inBubble(t, func(st *testing.T) {
		tt, end := gotestruntime.ExportOpenMethod(st, 30*time.Second, 0)
		waitOn(tt)
		end()
	})
	mustEqual(t, []string{name + ": t.Context() ended at the default 30s deadline; the suite declares no Timeout"}, lines)
}

func (s *DefaultDeadlineTestSuite) TestBeforeAllPastTheDefaultIsNamed(t *gotest.T) {
	name, lines := s.inBubble(t, func(st *testing.T) {
		gotestruntime.RunSetup(st, 30*time.Second, 0, waitOn)
	})
	mustEqual(t, []string{name + " BeforeAll: t.Context() ended at the default 30s deadline; the suite declares no SetupTimeout"}, lines)
}

func (s *DefaultDeadlineTestSuite) TestAfterAllPastTheDefaultIsNamed(t *gotest.T) {
	name, lines := s.inBubble(t, func(st *testing.T) {
		gotestruntime.RunTeardown(st, 30*time.Second, 0, waitOn)
	})
	mustEqual(t, []string{name + " AfterAll: t.Context() ended at the default 30s deadline; the suite declares no SetupTimeout"}, lines)
}

func (s *DefaultDeadlineTestSuite) TestNoLineForAPhaseThatEndedFirst(t *gotest.T) {
	_, lines := s.inBubble(t, func(st *testing.T) {
		_, end := gotestruntime.ExportOpenMethod(st, 30*time.Second, 0)
		end()
		gotestruntime.RunSetup(st, 30*time.Second, 0, func(*gotest.T) {})
		gotestruntime.RunTeardown(st, 30*time.Second, 0, func(*gotest.T) {})
	})
	mustEqual(t, []string{""}, lines)
}

func (s *DefaultDeadlineTestSuite) TestNoLineForADisabledDeadline(t *gotest.T) {
	_, lines := s.inBubble(t, func(st *testing.T) {
		_, end := gotestruntime.ExportOpenMethod(st, gotest.NoDeadline, gotest.NoDeadline)
		time.Sleep(time.Minute)
		end()
	})
	mustEqual(t, []string{""}, lines)
}

func (s *DefaultDeadlineTestSuite) TestNoLineOnAStop(t *gotest.T) {
	runstate.Stop()
	tt, end := gotestruntime.ExportOpenMethod(t.T(), 50*time.Millisecond, 0)
	waitOn(tt)
	time.Sleep(100 * time.Millisecond)
	end()
	mustEqual(t, "", s.out.String())
}
