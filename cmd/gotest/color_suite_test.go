package main_test

import (
	. "github.com/mvrahden/go-test/cmd/gotest"

	"github.com/mvrahden/go-test/pkg/gotest"
)

// ColorTestSuite: color is on only when nobody turned it off and stdout is a
// terminal; --no-color and NO_COLOR each turn it off.
// Sequential: it swaps the process-wide terminal probe and sets NO_COLOR.
type ColorTestSuite struct{}

func (s *ColorTestSuite) TestColorRule(t *gotest.T) {
	restore := ExportSetStdoutIsTerminal(func() bool { return true })
	defer restore()
	t.Setenv("NO_COLOR", "")

	t.When("stdout is a terminal", func(w *gotest.T) {
		w.It("colors unless --no-color", func(it *gotest.T) {
			gotest.False(it, ExportColorDisabled(false))
			gotest.True(it, ExportColorDisabled(true))
		})
		w.It("respects NO_COLOR", func(it *gotest.T) {
			it.Setenv("NO_COLOR", "1")
			gotest.True(it, ExportColorDisabled(false))
		})
	})
	t.When("stdout is not a terminal", func(w *gotest.T) {
		w.It("never colors", func(it *gotest.T) {
			restore := ExportSetStdoutIsTerminal(func() bool { return false })
			defer restore()
			gotest.True(it, ExportColorDisabled(false))
		})
	})
}

func (s *ColorTestSuite) TestFormatNames(t *gotest.T) {
	for it, tc := range gotest.Each(t, []struct {
		Desc   string
		format string
		ok     bool
	}{
		{"terminal", "terminal", true},
		{"md", "md", true},
		{"markdown", "markdown", true},
		{"json", "json", true},
		{"xml is unknown", "xml", false},
		{"empty is unknown", "", false},
	}) {
		err := ExportCheckFormat(tc.format)
		if tc.ok {
			gotest.NoError(it, err)
		} else {
			gotest.ErrorContains(it, err, "unknown --format")
		}
	}
}
