package gotest_test

import (
	"errors"
	"fmt"

	"github.com/mvrahden/go-test/pkg/gotest"
)

// MustTestSuite tests the Must wrapper that panics on assertion failure.
type MustTestSuite struct{}

func (s *MustTestSuite) TestMust(t *gotest.T) {
	t.When("ok is nil", func(w *gotest.T) {
		w.It("returns the value", func(it *gotest.T) {
			result := gotest.Must(42, nil)
			gotest.Equal(it, 42, result)
		})
	})

	t.When("ok is true", func(w *gotest.T) {
		w.It("returns the value", func(it *gotest.T) {
			result := gotest.Must("hello", true)
			gotest.Equal(it, "hello", result)
		})
	})

	t.When("ok is an error", func(w *gotest.T) {
		w.It("panics with the error message", func(it *gotest.T) {
			v := gotest.Panics(it, func() {
				gotest.Must(0, errors.New("boom"))
			})
			gotest.Contains(it, fmt.Sprint(v), "boom")
		})
	})

	t.When("ok is false", func(w *gotest.T) {
		w.It("panics with got-false message", func(it *gotest.T) {
			v := gotest.Panics(it, func() {
				gotest.Must(0, false)
			})
			gotest.Contains(it, fmt.Sprint(v), "false")
		})
	})

	t.When("used with multi-return expansion", func(w *gotest.T) {
		w.It("unwraps the value", func(it *gotest.T) {
			producer := func() (string, error) { return "value", nil }
			result := gotest.Must(producer())
			gotest.Equal(it, "value", result)
		})
	})

	t.When("ok is an unknown type", func(w *gotest.T) {
		w.It("panics with unsupported-type message", func(it *gotest.T) {
			v := gotest.Panics(it, func() {
				gotest.Must(42, 123)
			})
			gotest.Contains(it, fmt.Sprint(v), "unsupported")
		})
	})
}
