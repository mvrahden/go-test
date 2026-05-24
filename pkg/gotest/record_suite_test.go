package gotest_test

import (
	"github.com/mvrahden/go-test/pkg/gotest"
)

// RecordTestSuite tests the Record/R recorder for capturing assertion outcomes.
type RecordTestSuite struct{}

func (s *RecordTestSuite) TestRecord(t *gotest.T) {
	t.When("no assertions fail", func(w *gotest.T) {
		w.It("passes", func(it *gotest.T) {
			rec := gotest.Record(func(r *gotest.R) {})
			gotest.False(it, rec.Failed())
		})
	})

	t.When("Errorf is called", func(w *gotest.T) {
		w.It("marks as failed with formatted message", func(it *gotest.T) {
			rec := gotest.Record(func(r *gotest.R) {
				r.Errorf("expected %d, got %d", 1, 2)
			})
			gotest.True(it, rec.Failed())
			gotest.Equal(it, "expected 1, got 2", rec.Message())
		})
	})

	t.When("FailNow is called", func(w *gotest.T) {
		w.It("marks as failed", func(it *gotest.T) {
			rec := gotest.Record(func(r *gotest.R) {
				r.FailNow()
			})
			gotest.True(it, rec.Failed())
		})

		w.It("stops execution", func(it *gotest.T) {
			reached := false
			rec := gotest.Record(func(r *gotest.R) {
				r.FailNow()
				reached = true
			})
			gotest.True(it, rec.Failed())
			gotest.False(it, reached)
			gotest.Equal(it, "", rec.Message())
		})
	})

	t.When("Errorf then FailNow", func(w *gotest.T) {
		w.It("preserves the error message", func(it *gotest.T) {
			rec := gotest.Record(func(r *gotest.R) {
				r.Errorf("first error")
				r.FailNow()
			})
			gotest.True(it, rec.Failed())
			gotest.Equal(it, "first error", rec.Message())
		})
	})

	t.When("multiple Errorf calls", func(w *gotest.T) {
		w.It("keeps last message", func(it *gotest.T) {
			rec := gotest.Record(func(r *gotest.R) {
				r.Errorf("first")
				r.Errorf("second")
				r.Errorf("third")
			})
			gotest.True(it, rec.Failed())
			gotest.Equal(it, "third", rec.Message())
		})
	})

	t.When("Errorf with empty string", func(w *gotest.T) {
		w.It("marks as failed with empty message", func(it *gotest.T) {
			rec := gotest.Record(func(r *gotest.R) {
				r.Errorf("")
			})
			gotest.True(it, rec.Failed())
			gotest.Equal(it, "", rec.Message())
		})
	})
}
