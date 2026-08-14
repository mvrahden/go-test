package gotestspec

import (
	"fmt"
	"io"
	"math"
	"strings"
	"time"
)

type colors struct {
	reset, red, green, yellow, bold, dim string
}

var ansiColors = colors{
	reset:  "\033[0m",
	red:    "\033[31m",
	green:  "\033[32m",
	yellow: "\033[33m",
	bold:   "\033[1m",
	dim:    "\033[2m",
}

var noColors = colors{}

type renderConfig struct {
	color       bool
	coverage    *CoverageReport
	elapsed     time.Duration
	benchDeltas []BenchDelta
}

type RenderOption func(*renderConfig)

func WithNoColor() RenderOption {
	return func(c *renderConfig) { c.color = false }
}

func WithCoverage(report *CoverageReport) RenderOption {
	return func(c *renderConfig) { c.coverage = report }
}

func WithElapsed(d time.Duration) RenderOption {
	return func(c *renderConfig) { c.elapsed = d }
}

// BenchDelta is one benchmark's old-vs-new comparison, as rendered by
// RenderSummary and RenderMarkdownSummary via WithBenchDeltas. It mirrors
// gotestbench.Delta's shape but lives here (rather than being consumed
// directly) so that gotestspec never has to import gotestbench — callers
// (cmd/gotest/bench.go) convert []gotestbench.Delta to []BenchDelta at the
// call site. Filtering (e.g. significant-only vs. every row for -v) is
// also the caller's responsibility: the renderers show exactly the rows
// they're given.
type BenchDelta struct {
	Key           string // "pkg Suite/Name", matching gotestbench.Delta.Key
	OldNs, NewNs  float64
	PercentChange float64
	Significant   bool
}

// WithBenchDeltas attaches a benchmark old-vs-new comparison table to be
// rendered by RenderTerminal, RenderSummary, and RenderMarkdownSummary. A
// nil or empty slice renders no table.
func WithBenchDeltas(deltas []BenchDelta) RenderOption {
	return func(c *renderConfig) { c.benchDeltas = deltas }
}

func RenderTerminal(w io.Writer, packages []*Package, opts ...RenderOption) {
	cfg := renderConfig{color: true}
	for _, o := range opts {
		o(&cfg)
	}
	c := ansiColors
	if !cfg.color {
		c = noColors
	}

	multiPkg := len(packages) > 1

	for i, pkg := range packages {
		if multiPkg {
			if i > 0 {
				fmt.Fprintln(w)
			}
			fmt.Fprintf(w, "%s=== %s ===%s\n", c.dim, pkg.Path, c.reset)
			fmt.Fprintln(w)
		}
		for _, node := range pkg.Nodes {
			renderNode(w, node, 0, &c)
		}
		// A verdict that sits on the package itself — a build failure, a death
		// outside any test — has no node to render it; it must still show
		// beside its diagnostics, or the spec reads green next to a red exit.
		if PkgFailedOnItsOwn(pkg) {
			icon, clr := statusIcon(StatusFail, &c)
			fmt.Fprintf(w, "%s%s%s %sFAIL%s\n", clr, icon, c.reset, c.bold+c.red, c.reset)
			lines := filterOutput(pkg.Output)
			if len(lines) == 0 {
				lines = []string{noDiagnosticNote}
			}
			renderErrorOutput(w, lines, 1, &c)
		}
	}

	renderBenchDeltaTable(w, cfg.benchDeltas, c)
	fmt.Fprintln(w)
	stats := CollectStats(packages)
	renderSummary(w, stats, c)
}

func renderNode(w io.Writer, n *Node, depth int, c *colors) {
	indent := strings.Repeat("  ", depth)
	isLeaf := len(n.Children) == 0

	if isLeaf {
		icon, clr := statusIcon(n.Status, c)

		if n.Kind == KindBenchmark && n.Iterations > 0 {
			fmt.Fprintf(w, "%s%s%s%s %s  %s ns/op · %d B/op · %d allocs/op%s\n",
				indent, clr, icon, c.reset,
				n.Display, formatNs(n.NsPerOp), n.BytesPerOp, n.AllocsPerOp, c.reset)

			if n.Status == StatusFail {
				renderErrorOutput(w, n.Output, depth+2, c)
			}
			return
		}

		dur := formatDuration(n.Duration)

		suffix := ""
		if n.Excluded || n.Status == StatusSkip {
			suffix = " — SKIPPED"
		}

		fmt.Fprintf(w, "%s%s%s%s %s%s %s(%s)%s\n",
			indent, clr, icon, c.reset,
			n.Display, suffix,
			c.dim, dur, c.reset)

		if n.Status == StatusFail {
			renderErrorOutput(w, n.Output, depth+2, c)
		}
		return
	}

	label := n.Display
	if n.Kind == KindSuite || n.Kind == KindFixture || n.Kind == KindMethod || n.Kind == KindTest || n.Kind == KindBenchmark {
		label = c.bold + label + c.reset
	}

	suffix := ""
	if n.External {
		suffix = fmt.Sprintf(" %s— EXTERNAL%s", c.dim, c.reset)
	}
	if n.Focused {
		suffix = fmt.Sprintf(" %s— FOCUSED%s", c.yellow, c.reset)
	} else if n.Excluded {
		suffix = fmt.Sprintf(" %s— SKIPPED%s", c.yellow, c.reset)
	}

	// A node that failed on its own account must show a mark even when it left
	// no output — a bare t.Fail() otherwise renders as a green line beside a
	// red exit code.
	ownFailure := hasOwnDiagnostic(n) || failedOnItsOwn(n)
	if ownFailure {
		icon, clr := statusIcon(StatusFail, c)
		suffix += fmt.Sprintf(" %s%s%s", clr, icon, c.reset)
	}

	fmt.Fprintf(w, "%s%s%s\n", indent, label, suffix)

	if ownFailure {
		if len(filterOutput(n.Output)) > 0 {
			renderErrorOutput(w, n.Output, depth+1, c)
		} else {
			renderErrorOutput(w, []string{noDiagnosticNote}, depth+1, c)
		}
	}

	for _, child := range n.Children {
		renderNode(w, child, depth+1, c)
	}
}

func statusIcon(s Status, c *colors) (string, string) {
	switch s {
	case StatusPass:
		return "✓", c.green
	case StatusFail:
		return "✗", c.red
	case StatusSkip:
		return "~", c.yellow
	default:
		return "?", c.dim
	}
}

// formatNs renders a ns/op value the way Go's own benchmark output does:
// no trailing decimal for whole numbers ("1243"), one decimal place
// otherwise ("985.2").
func formatNs(ns float64) string {
	if ns == math.Trunc(ns) {
		return fmt.Sprintf("%.0f", ns)
	}
	return fmt.Sprintf("%.1f", ns)
}

func formatDuration(d time.Duration) string {
	ms := d.Milliseconds()
	if ms < 1 {
		return "<1ms"
	}
	if ms < 1000 {
		return fmt.Sprintf("%dms", ms)
	}
	return fmt.Sprintf("%.1fs", d.Seconds())
}

func renderErrorOutput(w io.Writer, output []string, depth int, c *colors) {
	indent := strings.Repeat("  ", depth)
	for _, line := range filterOutput(output) {
		fmt.Fprintf(w, "%s%s%s%s\n", indent, c.red, line, c.reset)
	}
}

func filterOutput(output []string) []string {
	var lines []string
	for _, line := range output {
		stripped := strings.TrimRight(line, " \t\n\r")
		trimmed := strings.TrimSpace(stripped)
		if trimmed == "" {
			continue
		}
		if strings.HasPrefix(trimmed, "=== ") || strings.HasPrefix(trimmed, "--- ") {
			continue
		}
		lines = append(lines, stripped)
	}
	if len(lines) == 0 {
		return nil
	}

	minIndent := len(lines[0]) - len(strings.TrimLeft(lines[0], " \t"))
	for _, line := range lines[1:] {
		indent := len(line) - len(strings.TrimLeft(line, " \t"))
		if indent < minIndent {
			minIndent = indent
		}
	}
	if minIndent == 0 {
		return lines
	}

	filtered := make([]string, len(lines))
	for i, line := range lines {
		filtered[i] = line[minIndent:]
	}
	return filtered
}

// renderBenchDeltaTable renders the "old vs new ns/op" comparison table in
// the same column format `gotest bench --against` established
// (cmd/gotest/bench.go's printBenchDeltaTable), so the table looks
// identical whether it's driven directly by the bench command or via a
// spec/summary render pass. deltas is rendered as given — filtering
// significant-only vs. every row (-v) is the caller's responsibility (see
// WithBenchDeltas). No-ops when deltas is empty.
func renderBenchDeltaTable(w io.Writer, deltas []BenchDelta, c colors) {
	// nil (WithBenchDeltas never called) means "no comparison happened at
	// all" -> no-op. An empty-but-non-nil slice (a comparison ran but every
	// row was filtered out, e.g. no significant deltas without -v) still
	// prints the header, so the reader can see a comparison was attempted
	// even when it found nothing worth flagging.
	if deltas == nil {
		return
	}
	fmt.Fprintln(w)
	fmt.Fprintf(w, "%sBENCHMARK  OLD ns/op  NEW ns/op  Δ%s\n", c.bold, c.reset)
	for _, d := range deltas {
		sign := ""
		if d.PercentChange >= 0 {
			sign = "+"
		}
		warn, clr, reset := "", "", ""
		if d.Significant && d.PercentChange > 0 {
			warn, clr, reset = " ⚠", c.red, c.reset
		}
		fmt.Fprintf(w, "%s%s  %.1f  %.1f  %s%.1f%%%s%s\n",
			clr, d.Key, d.OldNs, d.NewNs, sign, d.PercentChange, warn, reset)
	}
}

func renderSummary(w io.Writer, stats Stats, c colors) { //nolint:gocritic // hugeParam: stable API
	var parts []string
	if stats.Passed > 0 {
		parts = append(parts, fmt.Sprintf("%s%d passed%s", c.green, stats.Passed, c.reset))
	}
	if stats.Failed > 0 {
		parts = append(parts, fmt.Sprintf("%s%d failed%s", c.red, stats.Failed, c.reset))
	}
	if stats.Skipped > 0 {
		parts = append(parts, fmt.Sprintf("%s%d skipped%s", c.yellow, stats.Skipped, c.reset))
	}
	if stats.FailedPackages > 0 {
		parts = append(parts, fmt.Sprintf("%s%d failed packages%s", c.red, stats.FailedPackages, c.reset))
	}

	var counts []string
	if stats.Suites > 0 {
		counts = append(counts, fmt.Sprintf("%d suites", stats.Suites))
	}
	if stats.Behaviors > 0 {
		counts = append(counts, fmt.Sprintf("%d behaviors", stats.Behaviors))
	}
	if stats.Tests > 0 {
		counts = append(counts, fmt.Sprintf("%d stdlib tests", stats.Tests))
	}
	if stats.Benchmarks > 0 {
		counts = append(counts, fmt.Sprintf("%d benchmarks", stats.Benchmarks))
	}
	if len(counts) == 0 {
		counts = append(counts, "0 suites")
	}

	fmt.Fprintf(w, "%s: %s\n", strings.Join(counts, ", "), strings.Join(parts, ", "))
}
