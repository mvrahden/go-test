package gotest_test

import (
	"fmt"
	"regexp"
	"runtime"
	"strconv"
	"time"

	"github.com/mvrahden/go-test/pkg/gotest"
)

// --- spy infrastructure (in THIS file so the goroutine frame is in the suite file) ---

type spyT struct {
	msg    string
	failed bool
}

func (s *spyT) Errorf(format string, args ...any) {
	s.msg = fmt.Sprintf(format, args...)
	s.failed = true
}

func (s *spyT) FailNow() {
	s.failed = true
	runtime.Goexit()
}

func runSpy(fn func(t *spyT)) *spyT {
	spy := &spyT{}
	done := make(chan struct{})
	go func() {
		defer close(done)
		fn(spy)
	}()
	<-done
	return spy
}

var pollCountRe = regexp.MustCompile(`\((\d+) polls\)`)

func maskPollCount(msg string) (masked string, count int) {
	m := pollCountRe.FindStringSubmatch(msg)
	if m == nil {
		return msg, 0
	}
	count, _ = strconv.Atoi(m[1])
	return pollCountRe.ReplaceAllString(msg, "(N polls)"), count
}

// --- suite ---

// LineReportingTestSuite verifies that assertion failures report the correct
// call site — the outermost user frame, not gotest internals or intermediate
// helpers.
//
// Helpers live in linereport_helpers_test.go so that CallerFrame output
// distinguishes "resolved to test file" (this file) from "resolved to helper
// file" by filename.
type LineReportingTestSuite struct{}

func (s *LineReportingTestSuite) SuiteConfig() gotest.SuiteConfig {
	return gotest.SuiteConfig{Parallel: true}
}

// --- Direct assertions (no helper chain) ---

func (s *LineReportingTestSuite) TestDirectAssertion(t *gotest.T) {
	t.When("called via *R (Record)", func(w *gotest.T) {
		w.It("reports this file as the call site", func(it *gotest.T) {
			rec := gotest.Record(func(r *gotest.R) {
				gotest.Equal(r, 1, 2)
			})
			gotest.True(it, rec.Failed())
			gotest.MatchSnapshot(it, rec.Message())
		})
	})

	t.When("called via spyT (no Helper method)", func(w *gotest.T) {
		w.It("reports this file as the call site", func(it *gotest.T) {
			spy := runSpy(func(t *spyT) {
				gotest.Equal(t, 1, 2)
			})
			gotest.True(it, spy.failed)
			gotest.MatchSnapshot(it, spy.msg)
		})
	})
}

// --- Single helper ---

func (s *LineReportingTestSuite) TestSingleHelper(t *gotest.T) {
	t.When("called via *R through a helper in another file", func(w *gotest.T) {
		w.It("reports this file not the helper file", func(it *gotest.T) {
			rec := gotest.Record(func(r *gotest.R) {
				equalHelper(r, 1, 2)
			})
			gotest.True(it, rec.Failed())
			gotest.MatchSnapshot(it, rec.Message())
		})
	})

	t.When("called via spyT through a helper in another file", func(w *gotest.T) {
		w.It("reports this file not the helper file", func(it *gotest.T) {
			spy := runSpy(func(t *spyT) {
				equalHelper(t, 1, 2)
			})
			gotest.True(it, spy.failed)
			gotest.MatchSnapshot(it, spy.msg)
		})
	})
}

// --- Nested helpers ---

func (s *LineReportingTestSuite) TestNestedHelpers(t *gotest.T) {
	t.When("called via *R through two levels of helpers", func(w *gotest.T) {
		w.It("reports this file as the outermost call site", func(it *gotest.T) {
			rec := gotest.Record(func(r *gotest.R) {
				nestedHelper(r, 1, 2)
			})
			gotest.True(it, rec.Failed())
			gotest.MatchSnapshot(it, rec.Message())
		})
	})

	t.When("called via spyT through two levels of helpers", func(w *gotest.T) {
		w.It("reports this file as the outermost call site", func(it *gotest.T) {
			spy := runSpy(func(t *spyT) {
				nestedHelper(t, 1, 2)
			})
			gotest.True(it, spy.failed)
			gotest.MatchSnapshot(it, spy.msg)
		})
	})
}

// --- Eventually ---

func (s *LineReportingTestSuite) TestEventually(t *gotest.T) {
	t.When("inner assertion fails (via *R)", func(w *gotest.T) {
		w.It("timeout includes inner assertion call site", func(it *gotest.T) {
			spy := runSpy(func(t *spyT) {
				gotest.Eventually(t, 50*time.Millisecond, 10*time.Millisecond, func(poll *gotest.R) {
					gotest.True(poll, false, "condition not met")
				})
			})
			gotest.True(it, spy.failed)
			masked, polls := maskPollCount(spy.msg)
			gotest.MatchSnapshot(it, masked)
			gotest.InDelta(it, 5, polls, 1)
		})
	})

	t.When("called directly from test (no helper)", func(w *gotest.T) {
		w.It("timeout references this file", func(it *gotest.T) {
			spy := runSpy(func(t *spyT) {
				gotest.Eventually(t, 50*time.Millisecond, 10*time.Millisecond, func(poll *gotest.R) {
					gotest.True(poll, false)
				})
			})
			gotest.True(it, spy.failed)
			masked, polls := maskPollCount(spy.msg)
			gotest.MatchSnapshot(it, masked)
			gotest.InDelta(it, 5, polls, 1)
		})
	})
}

// --- Helper calling Eventually (the WaitForStatus pattern) ---

func (s *LineReportingTestSuite) TestHelperCallingEventually(t *gotest.T) {
	t.When("Eventually times out inside a helper", func(w *gotest.T) {
		w.It("traces back to this file not the helper", func(it *gotest.T) {
			spy := runSpy(func(t *spyT) {
				eventuallyHelper(t)
			})
			gotest.True(it, spy.failed)
			masked, polls := maskPollCount(spy.msg)
			gotest.MatchSnapshot(it, masked)
			gotest.InDelta(it, 5, polls, 1)
		})
	})
}

// --- Consistently ---

func (s *LineReportingTestSuite) TestConsistently(t *gotest.T) {
	t.When("assertion fails during polling", func(w *gotest.T) {
		w.It("failure references this file", func(it *gotest.T) {
			spy := runSpy(func(t *spyT) {
				count := 0
				gotest.Consistently(t, 50*time.Millisecond, 10*time.Millisecond, func(poll *gotest.R) {
					count++
					if count > 2 {
						gotest.True(poll, false, "broke on poll %d", count)
					}
				})
			})
			gotest.True(it, spy.failed)
			gotest.MatchSnapshot(it, spy.msg)
		})
	})
}
