package gotest_test

import (
	"bytes"
	"encoding/json"
	"fmt"
	"regexp"
	"time"

	"github.com/mvrahden/go-test/pkg/gotest"
)

// ShapeAssertionsTestSuite covers assertions on shapes: regular expressions, JSON, time and
// panics.
type ShapeAssertionsTestSuite struct{}

func (s *ShapeAssertionsTestSuite) SuiteConfig() gotest.SuiteConfig {
	cfg := gotest.DefaultSuiteConfig()
	cfg.Parallel = true
	return cfg
}

func (s *ShapeAssertionsTestSuite) TestRegexp(t *gotest.T) {
	t.When("string matches pattern", func(w *gotest.T) {
		w.It("passes for string pattern", func(it *gotest.T) {
			m := gotest.Record(func(r *gotest.R) { gotest.Regexp(r, `^\d+$`, "12345") })
			gotest.False(it, m.Failed())
		})
		w.It("passes for compiled regexp", func(it *gotest.T) {
			re := regexp.MustCompile(`hello`)
			m := gotest.Record(func(r *gotest.R) { gotest.Regexp(r, re, "say hello world") })
			gotest.False(it, m.Failed())
		})
		w.It("passes for empty pattern (matches everything)", func(it *gotest.T) {
			m := gotest.Record(func(r *gotest.R) { gotest.Regexp(r, ``, "anything") })
			gotest.False(it, m.Failed())
		})
	})

	t.When("string does not match pattern", func(w *gotest.T) {
		w.It("fails for string pattern", func(it *gotest.T) {
			m := gotest.Record(func(r *gotest.R) { gotest.Regexp(r, `^\d+$`, "abc") })
			gotest.True(it, m.Failed())
		})
		w.It("fails for compiled regexp", func(it *gotest.T) {
			re := regexp.MustCompile(`^hello$`)
			m := gotest.Record(func(r *gotest.R) { gotest.Regexp(r, re, "say hello world") })
			gotest.True(it, m.Failed())
		})
	})

	t.When("pattern is invalid", func(w *gotest.T) {
		w.It("fails", func(it *gotest.T) {
			m := gotest.Record(func(r *gotest.R) { gotest.Regexp(r, `[invalid`, "test") })
			gotest.True(it, m.Failed())
		})
	})

	t.When("regexp is nil", func(w *gotest.T) {
		w.It("fails with nil regexp error", func(it *gotest.T) {
			m := gotest.Record(func(r *gotest.R) { gotest.Regexp(r, (*regexp.Regexp)(nil), "anything") })
			gotest.True(it, m.Failed())
			gotest.Contains(it, m.Message(), "regexp is nil")
		})
	})
}

func (s *ShapeAssertionsTestSuite) TestJSONEq(t *gotest.T) {
	t.When("JSON structures are equal", func(w *gotest.T) {
		w.It("passes for different key order", func(it *gotest.T) {
			m := gotest.Record(func(r *gotest.R) { gotest.JSONEq(r, `{"a":1,"b":2}`, `{"b":2,"a":1}`) })
			gotest.False(it, m.Failed())
		})
		w.It("passes for []byte input", func(it *gotest.T) {
			m := gotest.Record(func(r *gotest.R) { gotest.JSONEq(r, []byte(`{"x":10}`), []byte(`{"x":10}`)) })
			gotest.False(it, m.Failed())
		})
		w.It("passes for marshalable struct", func(it *gotest.T) {
			type S struct {
				A int `json:"a"`
			}
			m := gotest.Record(func(r *gotest.R) { gotest.JSONEq(r, S{A: 5}, `{"a":5}`) })
			gotest.False(it, m.Failed())
		})
		w.It("passes for io.Reader input", func(it *gotest.T) {
			m := gotest.Record(func(r *gotest.R) {
				gotest.JSONEq(r, bytes.NewReader([]byte(`{"a":1,"b":2}`)), `{"b":2,"a":1}`)
			})
			gotest.False(it, m.Failed())
		})
		w.It("passes for json.RawMessage input", func(it *gotest.T) {
			m := gotest.Record(func(r *gotest.R) { gotest.JSONEq(r, json.RawMessage(`{"x":10}`), `{"x":10}`) })
			gotest.False(it, m.Failed())
		})
		w.It("passes for nested objects with different key order", func(it *gotest.T) {
			m := gotest.Record(func(r *gotest.R) {
				gotest.JSONEq(r, `{"a":{"x":1,"y":2},"b":3}`, `{"b":3,"a":{"y":2,"x":1}}`)
			})
			gotest.False(it, m.Failed())
		})
		w.It("passes for equal arrays", func(it *gotest.T) {
			m := gotest.Record(func(r *gotest.R) { gotest.JSONEq(r, `[1,2,3]`, `[1,2,3]`) })
			gotest.False(it, m.Failed())
		})
	})

	t.When("JSON structures differ", func(w *gotest.T) {
		w.It("fails for different values", func(it *gotest.T) {
			m := gotest.Record(func(r *gotest.R) { gotest.JSONEq(r, `{"a":1}`, `{"a":2}`) })
			gotest.True(it, m.Failed())
		})
		w.It("fails for different keys", func(it *gotest.T) {
			m := gotest.Record(func(r *gotest.R) { gotest.JSONEq(r, `{"a":1}`, `{"b":1}`) })
			gotest.True(it, m.Failed())
		})
		w.It("fails for null vs empty object", func(it *gotest.T) {
			m := gotest.Record(func(r *gotest.R) { gotest.JSONEq(r, `null`, `{}`) })
			gotest.True(it, m.Failed())
		})
		w.It("fails for arrays with different order", func(it *gotest.T) {
			m := gotest.Record(func(r *gotest.R) { gotest.JSONEq(r, `[1,2,3]`, `[3,2,1]`) })
			gotest.True(it, m.Failed())
		})
	})

	t.When("input is invalid JSON", func(w *gotest.T) {
		w.It("fails for invalid expected", func(it *gotest.T) {
			m := gotest.Record(func(r *gotest.R) { gotest.JSONEq(r, `{not json}`, `{"a":1}`) })
			gotest.True(it, m.Failed())
		})
		w.It("fails for invalid actual", func(it *gotest.T) {
			m := gotest.Record(func(r *gotest.R) { gotest.JSONEq(r, `{"a":1}`, `{not json}`) })
			gotest.True(it, m.Failed())
		})
	})
}

func (s *ShapeAssertionsTestSuite) TestTimeWithin(t *gotest.T) {
	t.When("times are within tolerance", func(w *gotest.T) {
		w.It("passes", func(it *gotest.T) {
			base := time.Now()
			m := gotest.Record(func(r *gotest.R) {
				gotest.TimeWithin(r, base, base.Add(50*time.Millisecond), 100*time.Millisecond)
			})
			gotest.False(it, m.Failed())
		})
		w.It("passes when actual is before expected", func(it *gotest.T) {
			base := time.Now()
			m := gotest.Record(func(r *gotest.R) {
				gotest.TimeWithin(r, base, base.Add(-50*time.Millisecond), 100*time.Millisecond)
			})
			gotest.False(it, m.Failed())
		})
		w.It("passes at exact tolerance boundary", func(it *gotest.T) {
			base := time.Now()
			m := gotest.Record(func(r *gotest.R) {
				gotest.TimeWithin(r, base, base.Add(100*time.Millisecond), 100*time.Millisecond)
			})
			gotest.False(it, m.Failed())
		})
	})

	t.When("times exceed tolerance", func(w *gotest.T) {
		w.It("fails", func(it *gotest.T) {
			base := time.Now()
			m := gotest.Record(func(r *gotest.R) {
				gotest.TimeWithin(r, base, base.Add(200*time.Millisecond), 100*time.Millisecond)
			})
			gotest.True(it, m.Failed())
		})
	})

	t.When("tolerance is negative", func(w *gotest.T) {
		w.It("always fails even for identical times", func(it *gotest.T) {
			base := time.Now()
			m := gotest.Record(func(r *gotest.R) {
				gotest.TimeWithin(r, base, base, -1*time.Second)
			})
			gotest.True(it, m.Failed())
		})
	})

	t.When("times are identical", func(w *gotest.T) {
		w.It("passes with any positive tolerance", func(it *gotest.T) {
			base := time.Now()
			m := gotest.Record(func(r *gotest.R) {
				gotest.TimeWithin(r, base, base, 1*time.Nanosecond)
			})
			gotest.False(it, m.Failed())
		})
	})
}

func (s *ShapeAssertionsTestSuite) TestTimeIsNow(t *gotest.T) {
	t.When("time is recent", func(w *gotest.T) {
		w.It("passes", func(it *gotest.T) {
			m := gotest.Record(func(r *gotest.R) { gotest.TimeIsNow(r, time.Now(), time.Second) })
			gotest.False(it, m.Failed())
		})
	})

	t.When("time is old", func(w *gotest.T) {
		w.It("fails", func(it *gotest.T) {
			m := gotest.Record(func(r *gotest.R) { gotest.TimeIsNow(r, time.Now().Add(-time.Hour), time.Second) })
			gotest.True(it, m.Failed())
		})
	})
}

func (s *ShapeAssertionsTestSuite) TestPanics(t *gotest.T) {
	t.When("function panics", func(w *gotest.T) {
		w.It("passes and returns recovered value", func(it *gotest.T) {
			var v any
			m := gotest.Record(func(r *gotest.R) {
				v = gotest.Panics(r, func() { panic("oh no") })
			})
			gotest.False(it, m.Failed())
			gotest.Equal(it, "oh no", v)
		})
		w.It("passes for non-string panic value", func(it *gotest.T) {
			var v any
			m := gotest.Record(func(r *gotest.R) {
				v = gotest.Panics(r, func() { panic(42) })
			})
			gotest.False(it, m.Failed())
			gotest.Equal(it, 42, v)
		})
		w.It("passes for nil panic", func(it *gotest.T) {
			m := gotest.Record(func(r *gotest.R) {
				gotest.Panics(r, func() { panic(nil) })
			})
			gotest.False(it, m.Failed())
		})
		w.It("passes for error panic", func(it *gotest.T) {
			var v any
			m := gotest.Record(func(r *gotest.R) {
				v = gotest.Panics(r, func() { panic(fmt.Errorf("boom")) })
			})
			gotest.False(it, m.Failed())
			gotest.Contains(it, fmt.Sprint(v), "boom")
		})
	})

	t.When("function does not panic", func(w *gotest.T) {
		w.It("fails", func(it *gotest.T) {
			m := gotest.Record(func(r *gotest.R) { gotest.Panics(r, func() {}) })
			gotest.True(it, m.Failed())
		})
	})
}
