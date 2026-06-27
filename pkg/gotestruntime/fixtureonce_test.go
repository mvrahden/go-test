package gotestruntime_test

import (
	"errors"
	"sync"
	"sync/atomic"

	"github.com/mvrahden/go-test/pkg/gotest"
	"github.com/mvrahden/go-test/pkg/gotestruntime"
)

type FixtureOnceTestSuite struct{}

func (s *FixtureOnceTestSuite) TestDo(t *gotest.T) {
	t.When("called once with nil error", func(w *gotest.T) {
		w.It("returns nil", func(it *gotest.T) {
			var fo gotestruntime.FixtureOnce
			err := fo.Do(func() error { return nil })
			gotest.NoError(it, err)
		})
	})

	t.When("called once with error", func(w *gotest.T) {
		w.It("returns the error", func(it *gotest.T) {
			var fo gotestruntime.FixtureOnce
			expected := errors.New("setup failed")
			err := fo.Do(func() error { return expected })
			gotest.ErrorIs(it, err, expected)
		})
	})

	t.When("called twice after success", func(w *gotest.T) {
		w.It("runs fn only once and returns nil both times", func(it *gotest.T) {
			var fo gotestruntime.FixtureOnce
			calls := 0
			fn := func() error { calls++; return nil }

			err1 := fo.Do(fn)
			err2 := fo.Do(fn)

			gotest.NoError(it, err1)
			gotest.NoError(it, err2)
			gotest.Equal(it, 1, calls)
		})
	})

	t.When("called twice after failure", func(w *gotest.T) {
		w.It("runs fn only once and returns cached error", func(it *gotest.T) {
			var fo gotestruntime.FixtureOnce
			expected := errors.New("setup failed")
			calls := 0
			fn := func() error { calls++; return expected }

			err1 := fo.Do(fn)
			err2 := fo.Do(fn)

			gotest.ErrorIs(it, err1, expected)
			gotest.ErrorIs(it, err2, expected)
			gotest.Equal(it, 1, calls)
		})
	})

	t.When("called concurrently", func(w *gotest.T) {
		w.It("initializes exactly once", func(it *gotest.T) {
			var fo gotestruntime.FixtureOnce
			var calls atomic.Int32
			var wg sync.WaitGroup
			for range 20 {
				wg.Add(1)
				go func() {
					defer wg.Done()
					_ = fo.Do(func() error {
						calls.Add(1)
						return nil
					})
				}()
			}
			wg.Wait()
			gotest.Equal(it, int32(1), calls.Load())
		})
	})
}
