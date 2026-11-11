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

func widthTree(label string) []*gotestspec.Package {
	return []*gotestspec.Package{{
		Path: "example.com/p",
		Nodes: []*gotestspec.Node{{
			Kind: gotestspec.KindSuite, Display: "Suite",
			Children: []*gotestspec.Node{{Kind: gotestspec.KindBlock, Display: label, Status: gotestspec.StatusPass}},
		}},
	}}
}

// widthCtx is the terminal probe one test stubs; AfterEach restores it.
type widthCtx struct {
	terminal bool
	restore  func()
}

func (s *SpecWidthTestSuite) BeforeEach(_ *gotest.T) *widthCtx {
	ctx := &widthCtx{}
	ctx.restore = ExportSetStdoutIsTerminal(func() bool { return ctx.terminal })
	return ctx
}

func (s *SpecWidthTestSuite) AfterEach(_ *gotest.T, ctx *widthCtx) { ctx.restore() }

func render(label string) string {
	var buf bytes.Buffer
	ExportRenderSpec(&buf, widthTree(label), "terminal", true, nil)
	return buf.String()
}

func (s *SpecWidthTestSuite) TestARedirectedSpecIsNotWrapped(t *gotest.T, ctx *widthCtx) {
	ctx.terminal = false
	t.Setenv("COLUMNS", "20")
	out := render(longBehavior)
	gotest.Contains(t, out, "✓ "+longBehavior+"\n", "every behavior stays on one line")
}

func (s *SpecWidthTestSuite) TestASpecWrittenToATerminalWrapsAtItsColumns(t *gotest.T, ctx *widthCtx) {
	ctx.terminal = true
	t.When("COLUMNS is a positive integer", func(w *gotest.T) {
		w.Setenv("COLUMNS", "30")
		out := render(longBehavior)
		w.It("wraps every line within it", func(it *gotest.T) {
			for _, line := range strings.Split(out, "\n") {
				gotest.LessOrEqual(it, len([]rune(line)), 30, "line %q", line)
			}
			gotest.NotContains(it, out, longBehavior)
		})
	})
	t.When("COLUMNS is not usable", func(w *gotest.T) {
		w.Setenv("COLUMNS", "wide")
		short := strings.Repeat("word ", 18) + "end" // 93 columns
		long := strings.Repeat("word ", 30) + "end"  // 153 columns
		w.It("wraps at 100", func(it *gotest.T) {
			gotest.Contains(it, render(short), "✓ "+short+"\n")
			out := render(long)
			gotest.NotContains(it, out, long)
			for _, line := range strings.Split(out, "\n") {
				gotest.LessOrEqual(it, len([]rune(line)), 100, "line %q", line)
			}
		})
	})
}
