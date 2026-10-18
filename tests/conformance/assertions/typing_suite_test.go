package assertions_test

import (
	"errors"
	"io/fs"
	"math"
	"strings"

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
	t.When("Contains is asked for a nil key in a map keyed by an interface or pointer type", func(w *gotest.T) {
		w.It("finds the key when present and reports it absent otherwise", func(it *gotest.T) {
			for _, m := range []*gotest.R{
				gotest.Record(func(r *gotest.R) { gotest.Contains(r, map[any]int{nil: 1}, nil) }),
				gotest.Record(func(r *gotest.R) { gotest.Contains(r, map[*int]int{nil: 1}, nil) }),
				gotest.Record(func(r *gotest.R) { gotest.NotContains(r, map[any]int{"a": 1}, nil) }),
				gotest.Record(func(r *gotest.R) { gotest.NotContains(r, map[*int]int{new(int): 1}, nil) }),
			} {
				gotest.False(it, m.Failed(), m.Message())
			}
			miss := gotest.Record(func(r *gotest.R) { gotest.Contains(r, map[any]int{"a": 1}, nil) })
			gotest.True(it, miss.Failed())
			gotest.Contains(it, miss.Message(), "does not contain")
		})
	})
	t.When("Contains is asked for a nil key in a map whose key type cannot be nil", func(w *gotest.T) {
		w.It("fails naming the map's type", func(it *gotest.T) {
			m := gotest.Record(func(r *gotest.R) { gotest.Contains(r, map[string]int{"": 1}, nil) })
			gotest.True(it, m.Failed())
			gotest.Contains(it, m.Message(), "a map[string]int cannot hold a key of type <nil>")
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
			m := gotest.Record(func(r *gotest.R) { _ = gotest.ErrorAs[*fs.PathError](r, errors.New("plain")) })
			gotest.True(it, m.Failed())
			gotest.Contains(it, m.Message(), "is not a *fs.PathError")
		})
	})
}

func (s *AssertionTypingTestSuite) TestJSONEq(t *gotest.T) {
	t.When("JSONEq compares integers beyond float64's exact range", func(w *gotest.T) {
		w.It("tells 9007199254740993 from 9007199254740992", func(it *gotest.T) {
			m := gotest.Record(func(r *gotest.R) { gotest.JSONEq(r, `{"n":9007199254740993}`, `{"n":9007199254740992}`) })
			gotest.True(it, m.Failed())
			same := gotest.Record(func(r *gotest.R) { gotest.JSONEq(r, `{"n":9007199254740993}`, `{"n":9007199254740993}`) })
			gotest.False(it, same.Failed(), same.Message())
		})
	})
	t.When("JSONEq is given a value followed by a stray } or ]", func(w *gotest.T) {
		w.It("fails: the input is not valid JSON", func(it *gotest.T) {
			for _, pair := range [][2]string{{`{"a":1}}`, `{"a":1}`}, {`[1]]`, `[1]`}} {
				m := gotest.Record(func(r *gotest.R) { gotest.JSONEq(r, pair[0], pair[1]) })
				gotest.True(it, m.Failed(), pair[0])
				gotest.Contains(it, m.Message(), "could not normalize expected", pair[0])
				gotest.Contains(it, m.Message(), "trailing data after the value", pair[0])
			}
		})
	})
	t.When("JSONEq compares an integer with its decimal spelling", func(w *gotest.T) {
		w.It("1 equals 1.0", func(it *gotest.T) {
			m := gotest.Record(func(r *gotest.R) { gotest.JSONEq(r, `{"n":1}`, `{"n":1.0}`) })
			gotest.False(it, m.Failed(), m.Message())
			e := gotest.Record(func(r *gotest.R) { gotest.JSONEq(r, `[1e2]`, `[100]`) })
			gotest.False(it, e.Failed(), e.Message())
		})
	})
}

type lines []string

func (v lines) GoString() string { return strings.Join(v, "\n") }

func (s *AssertionTypingTestSuite) TestDiffs(t *gotest.T) {
	t.When("Equal fails on values whose rendering spans lines", func(w *gotest.T) {
		w.It("the message carries a unified diff: hunks of changes with three lines of context", func(it *gotest.T) {
			before := lines{"a", "b", "c", "d", "e", "f", "g", "h", "i", "j"}
			after := append(lines(nil), before...)
			after[5] = "F"
			m := gotest.Record(func(r *gotest.R) { gotest.Equal(r, before, after) })
			gotest.True(it, m.Failed())
			gotest.Contains(it, m.Message(), "@@ -3,7 +3,7 @@")
			gotest.Contains(it, m.Message(), "-f\n")
			gotest.Contains(it, m.Message(), "+F\n")
			gotest.NotContains(it, m.Message(), "@@ -1,", "the hunk began before its three lines of context")
		})
	})
}
