package assertions_test

import (
	"errors"
	"io/fs"
	"math"

	"github.com/mvrahden/go-test/pkg/gotest"
)

// AssertionTypingTestSuite: assertions take the types they are given at face
// value and say what they cannot work with.
type AssertionTypingTestSuite struct{}

func (s *AssertionTypingTestSuite) SuiteConfig() gotest.SuiteConfig {
	cfg := gotest.DefaultSuiteConfig()
	cfg.Parallel = true
	return cfg
}

type pattern string

type note string

func (s *AssertionTypingTestSuite) TestRegexp(t *gotest.T) {
	t.When("Regexp is given a pattern of a named string type", func(w *gotest.T) {
		w.It("compiles and matches it like a string", func(it *gotest.T) {
			m := gotest.Record(func(r *gotest.R) { gotest.Regexp(r, pattern(`^a+$`), "aaa") })
			gotest.False(it, m.Failed(), m.Message())
			miss := gotest.Record(func(r *gotest.R) { gotest.Regexp(r, pattern(`^a+$`), "b") })
			gotest.True(it, miss.Failed())
		})
	})
}

func (s *AssertionTypingTestSuite) TestContains(t *gotest.T) {
	t.When("Contains is given a needle no element of the haystack could be", func(w *gotest.T) {
		w.It("fails naming the haystack's type and the needle's", func(it *gotest.T) {
			m := gotest.Record(func(r *gotest.R) { gotest.Contains(r, []int{1, 2}, "x") })
			gotest.True(it, m.Failed())
			gotest.Contains(it, m.Message(), "a []int cannot hold a string")
			n := gotest.Record(func(r *gotest.R) { gotest.NotContains(r, map[string]int{"a": 1}, 7) })
			gotest.True(it, n.Failed())
			gotest.Contains(it, n.Message(), "a map[string]int cannot hold a key of type int")
		})
	})
	t.When("Contains compares named string kinds", func(w *gotest.T) {
		w.It("checks the substring as it does for strings", func(it *gotest.T) {
			m := gotest.Record(func(r *gotest.R) { gotest.Contains(r, note("hello world"), note("lo w")) })
			gotest.False(it, m.Failed(), m.Message())
			miss := gotest.Record(func(r *gotest.R) { gotest.Contains(r, note("hello"), "zzz") })
			gotest.True(it, miss.Failed())
			gotest.Contains(it, miss.Message(), "does not contain")
		})
	})
}

func (s *AssertionTypingTestSuite) TestInDelta(t *gotest.T) {
	t.When("InDelta is given a NaN delta", func(w *gotest.T) {
		w.It("fails: no distance is within NaN", func(it *gotest.T) {
			m := gotest.Record(func(r *gotest.R) { gotest.InDelta(r, 1.0, 1.0, math.NaN()) })
			gotest.True(it, m.Failed())
			gotest.Contains(it, m.Message(), "delta is NaN")
		})
	})
	t.When("InDelta compares two equal infinities", func(w *gotest.T) {
		w.It("they are within any delta", func(it *gotest.T) {
			m := gotest.Record(func(r *gotest.R) { gotest.InDelta(r, math.Inf(1), math.Inf(1), 0) })
			gotest.False(it, m.Failed(), m.Message())
			opposite := gotest.Record(func(r *gotest.R) { gotest.InDelta(r, math.Inf(1), math.Inf(-1), 1e300) })
			gotest.True(it, opposite.Failed())
		})
	})
}

func (s *AssertionTypingTestSuite) TestMessages(t *gotest.T) {
	t.When("an assertion's first extra argument is not a string", func(w *gotest.T) {
		w.It("every extra argument is in the message", func(it *gotest.T) {
			m := gotest.Record(func(r *gotest.R) { gotest.True(r, false, 42, "retries left") })
			gotest.Contains(it, m.Message(), "42 retries left")
		})
	})
	t.When("ErrorAs is given an error of another type", func(w *gotest.T) {
		w.It("names the type it expected", func(it *gotest.T) {
			m := gotest.Record(func(r *gotest.R) { gotest.ErrorAs[*fs.PathError](r, errors.New("plain")) })
			gotest.True(it, m.Failed())
			gotest.Contains(it, m.Message(), "is not a *fs.PathError")
		})
	})
}
