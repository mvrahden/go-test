package datarace

import (
	"sync"

	"github.com/mvrahden/go-test/pkg/gotest"
)

type CounterTestSuite struct {
	counter *Counter
}

func (s *CounterTestSuite) BeforeEach(t *gotest.T) {
	t.Skipf("run manually: demonstrates data race diagnostic")
	s.counter = &Counter{}
}

// TestConcurrentIncrement demonstrates a data race: multiple goroutines
// increment an unsynchronized counter. Run with -race to see the diagnostic.
func (s *CounterTestSuite) TestConcurrentIncrement(t *gotest.T) {
	t.When("multiple goroutines increment without synchronization", func(t *gotest.T) {
		var wg sync.WaitGroup
		for range 100 {
			wg.Add(1)
			go func() {
				defer wg.Done()
				s.counter.Increment()
			}()
		}
		wg.Wait()

		t.It("reaches 100", func(t *gotest.T) {
			gotest.Equal(t, 100, s.counter.Value())
		})
	})
}

type PanicTestSuite struct{}

func (s *PanicTestSuite) BeforeEach(t *gotest.T) {
	t.Skipf("run manually: demonstrates in-test panic diagnostic")
}

func (s *PanicTestSuite) TestOutOfBoundsAccess(t *gotest.T) {
	t.When("accessing an index beyond the slice length", func(t *gotest.T) {
		items := []string{"a", "b", "c"}

		t.It("panics", func(t *gotest.T) {
			got := NthElement(items, 5)
			gotest.Equal(t, "f", got)
		})
	})
}

type FixturePanicTestSuite struct {
	items []string
}

func (s *FixturePanicTestSuite) BeforeEach(t *gotest.T) {
	t.Skipf("run manually: demonstrates fixture panic diagnostic")
	s.items = NilSlice()
	_ = s.items[0]
}

func (s *FixturePanicTestSuite) TestAccess(t *gotest.T) {
	t.When("the fixture sets up items", func(t *gotest.T) {
		t.It("has elements", func(t *gotest.T) {
			gotest.NotEmpty(t, s.items)
		})
	})
}

type SafeCounterTestSuite struct {
	counter *SafeCounter
}

func (s *SafeCounterTestSuite) BeforeEach(t *gotest.T) {
	s.counter = &SafeCounter{}
}

func (s *SafeCounterTestSuite) TestConcurrentIncrement(t *gotest.T) {
	t.When("multiple goroutines increment with synchronization", func(t *gotest.T) {
		var wg sync.WaitGroup
		for range 100 {
			wg.Add(1)
			go func() {
				defer wg.Done()
				s.counter.Increment()
			}()
		}
		wg.Wait()

		t.It("reaches 100", func(t *gotest.T) {
			gotest.Equal(t, 100, s.counter.Value())
		})
	})
}
