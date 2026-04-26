package gotest_test

import (
	"errors"

	"github.com/mvrahden/go-test/pkg/gotest"
)

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
		w.It("panics", func(it *gotest.T) {
			gotest.Panics(it, func() {
				gotest.Must(0, errors.New("boom"))
			})
		})
	})

	t.When("ok is false", func(w *gotest.T) {
		w.It("panics", func(it *gotest.T) {
			gotest.Panics(it, func() {
				gotest.Must(0, false)
			})
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
		w.It("panics", func(it *gotest.T) {
			gotest.Panics(it, func() {
				gotest.Must(42, 123)
			})
		})
	})
}
