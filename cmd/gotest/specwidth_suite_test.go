package main_test

import (
	"bytes"
	"strings"

	. "github.com/mvrahden/go-test/cmd/gotest"
	"github.com/mvrahden/go-test/internal/gotestspec"
	"github.com/mvrahden/go-test/pkg/gotest"
)

// Sequential: Setenv.
// SpecWidthTestSuite covers the width the terminal spec wraps at: the
// terminal's, never a file's or a pipe's.
type SpecWidthTestSuite struct{}

const longBehavior = "a behavior whose label runs well past twenty columns"

func widthTree() []*gotestspec.Package {
	return []*gotestspec.Package{{
		Path: "example.com/p",
		Nodes: []*gotestspec.Node{{
			Kind: gotestspec.KindSuite, Display: "Suite",
			Children: []*gotestspec.Node{{Kind: gotestspec.KindBlock, Display: longBehavior, Status: gotestspec.StatusPass}},
		}},
	}}
}

func (s *SpecWidthTestSuite) render() string {
	var buf bytes.Buffer
	ExportRenderSpec(&buf, widthTree(), "terminal", true, nil)
	return buf.String()
}

func (s *SpecWidthTestSuite) TestARedirectedSpecIsNotWrapped(t *gotest.T) {
	restore := ExportSetStdoutIsTerminal(func() bool { return false })
	t.Setenv("COLUMNS", "20")
	out := s.render()
	restore()
	gotest.Contains(t, out, "✓ "+longBehavior+"\n", "every behavior stays on one line")
}

func (s *SpecWidthTestSuite) TestASpecWrittenToATerminalWrapsAtItsColumns(t *gotest.T) {
	t.When("COLUMNS is a positive integer", func(w *gotest.T) {
		restore := ExportSetStdoutIsTerminal(func() bool { return true })
		w.Setenv("COLUMNS", "30")
		out := s.render()
		restore()
		w.It("wraps every line within it", func(it *gotest.T) {
			for _, line := range strings.Split(out, "\n") {
				gotest.LessOrEqual(it, len([]rune(line)), 30, "line %q", line)
			}
			gotest.NotContains(it, out, longBehavior)
		})
	})
	t.When("COLUMNS is not usable", func(w *gotest.T) {
		restore := ExportSetStdoutIsTerminal(func() bool { return true })
		w.Setenv("COLUMNS", "wide")
		out := s.render()
		restore()
		w.It("wraps at 100", func(it *gotest.T) {
			gotest.Contains(it, out, "✓ "+longBehavior+"\n")
		})
	})
}
