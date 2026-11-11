package gotestspec

import (
	"fmt"
	"io"
	"math"
	"sort"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"
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
	color           bool
	coverage        *CoverageReport
	elapsed         time.Duration
	withoutVerdicts bool
	benchDeltas     []BenchDelta
	benchEnvWarning string
	benchGate       *BenchGate
	groups          []GroupRun
	hidden          map[Status]bool
	packageStats    bool
	module          string
	provenance      Provenance
	width           int
}

type RenderOption func(*renderConfig)

// GroupRun is what a named ExclusiveGroup took in a run: its suites and the
// wall time from its first suite's start to its last suite's end.
type GroupRun struct {
	Name   string
	Suites int
	Wall   time.Duration
}

// WithGroups adds one line per named group to a summary.
func WithGroups(groups []GroupRun) RenderOption {
	return func(c *renderConfig) { c.groups = groups }
}

// groupLine is a group's summary line: "group e2e: 6 suites, 1m12s, run apart".
func groupLine(g GroupRun) string {
	unit := "suites"
	if g.Suites == 1 {
		unit = "suite"
	}
	return fmt.Sprintf("group %s: %d %s, %s, run apart", g.Name, g.Suites, unit, formatDuration(g.Wall))
}

func WithNoColor() RenderOption {
	return func(c *renderConfig) { c.color = false }
}

func WithCoverage(report *CoverageReport) RenderOption {
	return func(c *renderConfig) { c.coverage = report }
}

func WithElapsed(d time.Duration) RenderOption {
	return func(c *renderConfig) { c.elapsed = d }
}

// WithoutVerdicts renders the tree as a specification rather than as a result:
// no status glyph and no duration. A statically derived spec has executed
// nothing, so showing "?" beside every behavior would invite the reader to
// think a verdict was expected and missing.
func WithoutVerdicts() RenderOption {
	return func(c *renderConfig) { c.withoutVerdicts = true }
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
	// InsufficientSample says a side had fewer samples than a significance
	// test needs, so Significant is a ±20% rule of thumb.
	InsufficientSample bool
}

// heuristicMark follows a delta row whose significance is a rule of thumb.
const heuristicMark = "(n<4, ±20% heuristic)"

// deltaMarks renders what follows a delta row's percentage: the regression
// warning and the heuristic mark, in that order.
func deltaMarks(d BenchDelta) string { //nolint:gocritic // hugeParam: stable API
	marks := ""
	if d.Significant && d.PercentChange > 0 {
		marks += " ⚠"
	}
	if d.InsufficientSample {
		marks += " " + heuristicMark
	}
	return marks
}

// WithBenchDeltas attaches a benchmark old-vs-new comparison table to be
// rendered by RenderTerminal, RenderSummary, and RenderMarkdownSummary. A
// nil or empty slice renders no table.
func WithBenchDeltas(deltas []BenchDelta) RenderOption {
	return func(c *renderConfig) { c.benchDeltas = deltas }
}

// BenchGate is the gate verdict of a bench run, mirroring gotestbench.Gate
// (which imports this package, so it cannot be used here directly).
type BenchGate struct {
	ThresholdPct float64
	WorstPct     float64
	WorstKey     string
	Breached     bool
}

// WithBenchEnvWarning attaches the one-line warning that the baseline came
// from another toolchain or platform to RenderMarkdownBenchSummary. An empty
// string renders nothing. The terminal says this on stderr, as the gate does.
func WithBenchEnvWarning(note string) RenderOption {
	return func(c *renderConfig) { c.benchEnvWarning = note }
}

// WithBenchGate attaches a gate verdict to RenderMarkdownBenchSummary.
// nil renders no verdict: no gate was set.
func WithBenchGate(gate *BenchGate) RenderOption {
	return func(c *renderConfig) { c.benchGate = gate }
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

	printed := 0
	for _, pkg := range packages {
		if !cfg.packageShown(pkg) {
			continue
		}
		if printed > 0 {
			fmt.Fprintln(w)
		}
		printed++
		name, _ := cfg.packageName(pkg)
		own := !cfg.withoutVerdicts && PkgFailedOnItsOwn(pkg) && !cfg.hidden[StatusFail]
		heading := name
		if own {
			heading += " — FAILED"
		}
		if !cfg.withoutVerdicts {
			if counts := countsPhrase(CollectStats([]*Package{pkg})); counts != "" {
				heading += " — " + counts
			}
		}
		lines := wrapLabel(heading, 8, 8, cfg.width) // room for "=== " and " ==="
		for i, l := range lines {
			lead, end := "    ", ""
			if i == 0 {
				lead = "=== "
			}
			if i == len(lines)-1 {
				end = " ==="
			}
			fmt.Fprintf(w, "%s%s%s%s%s\n", c.dim, lead, l, end, c.reset)
		}
		fmt.Fprintln(w)
		for _, node := range cfg.visible(pkg.Nodes) {
			renderNode(w, node, 0, &c, &cfg)
		}
		// A verdict that sits on the package itself — a build failure, a death
		// outside any test — has no node to render it; it must still show
		// beside its diagnostics, or the spec reads green next to a red exit.
		if own {
			icon, clr := statusIcon(StatusFail, &c)
			fmt.Fprintf(w, "%s%s%s %sFAIL%s\n", clr, icon, c.reset, c.bold+c.red, c.reset)
			renderErrorOutput(w, diagnostic(pkg.Output), 1, &c)
		}
	}

	renderBenchDeltaTable(w, cfg.benchDeltas, c)
	if !cfg.withoutVerdicts {
		renderAttention(w, attention(packages, &cfg), &c, &cfg)
	}
	if line := provenanceLine(packages, &cfg); line != "" {
		fmt.Fprintln(w)
		for _, l := range wrapLabel(line, 0, 0, cfg.width) {
			fmt.Fprintf(w, "%s%s%s\n", c.dim, l, c.reset)
		}
	}
	fmt.Fprintln(w)
	stats := cfg.stats(packages)
	renderSummary(w, stats, c)
}

// renderAttention lists the failures and the rows not run after the packages:
// each row's path under its mark, a failure's message beneath it.
func renderAttention(w io.Writer, rows []attentionRow, c *colors, cfg *renderConfig) {
	if len(rows) == 0 {
		return
	}
	fmt.Fprintln(w)
	fmt.Fprintf(w, "%sNeeds attention%s\n", c.bold, c.reset)
	for _, r := range rows {
		icon, clr := statusIcon(r.Status, c)
		writeRow(w, &rowLine{lead: clr + icon + c.reset + " ", leadWidth: 2, text: strings.Join(r.Path, " › ")}, c, cfg)
		renderErrorOutput(w, r.Output, 1, c)
	}
}

// tailPart is text that follows a row's label, with its color.
type tailPart struct{ text, color string }

// rowLine is one row: an optional lead (a mark), a label, and what follows it.
type rowLine struct {
	indent    int
	lead      string // as written, with color
	leadWidth int    // its visible columns
	text      string // the label, plain
	style     string // color of the label
	tail      []tailPart
}

// writeRow writes a row, its label wrapped at the configured width with the
// lines after the first under the label's start. The tail follows the last
// line, or takes a line of its own when it does not fit there.
func writeRow(w io.Writer, r *rowLine, c *colors, cfg *renderConfig) {
	hang := r.indent + max(r.leadWidth, 2)
	lines := wrapLabel(r.text, r.indent+r.leadWidth, hang, cfg.width)
	tail, tailWidth := "", 0
	for _, p := range r.tail {
		tail += p.color + p.text
		if p.color != "" {
			tail += c.reset
		}
		tailWidth += utf8.RuneCountInString(p.text)
	}
	for i, l := range lines {
		pad := strings.Repeat(" ", hang)
		if i == 0 {
			pad = strings.Repeat(" ", r.indent) + r.lead
		}
		styled := l
		if r.style != "" {
			styled = r.style + l + c.reset
		}
		fmt.Fprint(w, pad, styled)
		if i == len(lines)-1 && tail != "" {
			if cfg.width > 0 && hang+utf8.RuneCountInString(l)+tailWidth > cfg.width && i > 0 {
				fmt.Fprintf(w, "\n%s%s", strings.Repeat(" ", hang), strings.TrimLeft(tail, " "))
			} else {
				fmt.Fprint(w, tail)
			}
		}
		fmt.Fprintln(w)
	}
}

// bareSuffix annotates a node in a tree that has not run. Exclusion is a
// property of the declaration, so it is worth stating; "SKIPPED" is not, because
// nothing was there to skip. Incompleteness is stated for the same reason the
// walker records it: a partial list that presents itself as whole is worse than
// no list at all.
func bareSuffix(n *Node) string {
	suffix := ""
	if n.Excluded {
		suffix += " — EXCLUDED"
	}
	if n.Incomplete {
		suffix += " — INCOMPLETE: behaviors known only at run time"
	}
	return suffix
}

func renderNode(w io.Writer, n *Node, depth int, c *colors, cfg *renderConfig) {
	bare := cfg.withoutVerdicts
	indent := 2 * depth
	children := cfg.visible(n.Children)
	isLeaf := len(children) == 0

	mark := func(s Status) (string, int) {
		icon, clr := statusIcon(s, c)
		return clr + icon + c.reset + " ", 2
	}
	// A duration shows from one second up, and only beside a verdict.
	duration := func() []tailPart {
		if d := EffectiveDuration(n); d >= time.Second {
			return []tailPart{{" (" + formatDuration(d) + ")", c.dim}}
		}
		return nil
	}

	// A node without a verdict, declared and not reported or started and
	// never ended, took no time it can show; it says so instead.
	unrun := noVerdict(n) && !bare && (n.NotRun || n.Kind != KindBenchmark)
	if unrun && isLeaf {
		lead, lw := mark(StatusNone)
		writeRow(w, &rowLine{indent: indent, lead: lead, leadWidth: lw, text: n.Display,
			tail: []tailPart{{" — NOT RUN", c.dim}}}, c, cfg)
		return
	}

	// A passing fuzz target reads as one line: its seeds are evidence, not
	// behaviors. A failing one opens up so the offending entry is named.
	if !unrun && n.Kind == KindFuzz && !anyDescendantFailed(n) && !bare {
		lead, lw := mark(n.Status)
		tail := append([]tailPart{{fmt.Sprintf("  %d seeds", len(n.Children)), ""}}, duration()...)
		writeRow(w, &rowLine{indent: indent, lead: lead, leadWidth: lw, text: n.Display, tail: tail}, c, cfg)
		return
	}

	if isLeaf {
		if n.Kind == KindBenchmark && n.Iterations > 0 {
			lead, _ := mark(n.Status)
			fmt.Fprintf(w, "%s%s%s  %s ns/op · %d B/op · %d allocs/op%s%s\n",
				strings.Repeat(" ", indent), lead, n.Display, formatNs(n.NsPerOp), n.BytesPerOp, n.AllocsPerOp, formatMetrics(n.Metrics), c.reset)
			if n.Status == StatusFail {
				renderErrorOutput(w, n.Output, depth+2, c)
			}
			return
		}

		if bare {
			writeRow(w, &rowLine{indent: indent, text: n.Display, tail: []tailPart{{bareSuffix(n), ""}}}, c, cfg)
			return
		}

		var tail []tailPart
		if n.Excluded || n.Status == StatusSkip {
			tail = append(tail, tailPart{" — SKIPPED", ""})
		}
		lead, lw := mark(n.Status)
		writeRow(w, &rowLine{indent: indent, lead: lead, leadWidth: lw, text: n.Display, tail: append(tail, duration()...)}, c, cfg)

		if n.Status == StatusFail {
			renderErrorOutput(w, n.Output, depth+2, c)
		}
		return
	}

	row := rowLine{indent: indent, text: n.Display, style: branchStyle(n, c)}
	if n.External {
		row.tail = append(row.tail, tailPart{" — EXTERNAL", c.dim})
	}
	if n.Focused {
		row.tail = append(row.tail, tailPart{" — FOCUSED", c.yellow})
	} else if n.Excluded {
		row.tail = append(row.tail, tailPart{" — SKIPPED", c.yellow})
	}

	if bare {
		// Nothing ran, so the container says what it is rather than what
		// happened to it, and it says when its children are not the whole list.
		row.tail = append(row.tail, tailPart{bareSuffix(n), ""})
		writeRow(w, &row, c, cfg)
		for _, child := range children {
			renderNode(w, child, depth+1, c, cfg)
		}
		return
	}

	// A node that failed on its own account must show a mark even when it left
	// no output — a bare t.Fail() otherwise renders as a green line beside a
	// red exit code.
	ownFailure := !unrun && (hasOwnDiagnostic(n) || failedOnItsOwn(n))
	if ownFailure {
		icon, clr := statusIcon(StatusFail, c)
		row.tail = append(row.tail, tailPart{" " + icon, clr})
	}
	if n.Kind == KindSuite {
		// A suite line states its counts; its clock is the rows' to show.
		if counts := countsPhrase(nodeStats(n)); counts != "" {
			row.tail = append(row.tail, tailPart{" — " + counts, ""})
		}
	} else if !unrun {
		row.tail = append(row.tail, duration()...)
	}
	writeRow(w, &row, c, cfg)

	if ownFailure {
		renderErrorOutput(w, diagnostic(n.Output), depth+1, c)
	}

	for _, child := range children {
		renderNode(w, child, depth+1, c, cfg)
	}
}

// branchStyle is a branch's color, bold for what declares rather than
// describes: suites, fixtures, methods, tests, benchmarks and fuzz targets.
func branchStyle(n *Node, c *colors) string {
	if n.Kind == KindSuite || n.Kind == KindFixture || n.Kind == KindMethod || n.Kind == KindTest || n.Kind == KindBenchmark || n.Kind == KindFuzz {
		return c.bold
	}
	return ""
}

func statusIcon(s Status, c *colors) (string, string) {
	switch s {
	case StatusPass:
		return "✓", c.green
	case StatusFail:
		return "✗", c.red
	case StatusSkip:
		return "–", c.yellow
	default:
		return "○", c.dim
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
func renderBenchDeltaTable(w io.Writer, deltas []BenchDelta, c colors) { //nolint:gocritic // hugeParam: stable API
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
		clr, reset := "", ""
		if d.Significant && d.PercentChange > 0 {
			clr, reset = c.red, c.reset
		}
		fmt.Fprintf(w, "%s%s  %.1f  %.1f  %s%.1f%%%s%s\n",
			clr, d.Key, d.OldNs, d.NewNs, sign, d.PercentChange, deltaMarks(d), reset)
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
	if stats.NotRun > 0 {
		parts = append(parts, fmt.Sprintf("%s%d not run%s", c.dim, stats.NotRun, c.reset))
	}
	if stats.FailedPackages > 0 {
		parts = append(parts, fmt.Sprintf("%s%s%s", c.red, countNoun(stats.FailedPackages, "failed package", "failed packages"), c.reset))
	}

	counts := countParts(stats)

	if len(parts) == 0 {
		// A spec that has not run has counts but no verdicts. Printing the
		// colon anyway reads as a line that lost its tail.
		fmt.Fprintf(w, "%s\n", strings.Join(counts, ", "))
		return
	}
	fmt.Fprintf(w, "%s: %s\n", strings.Join(counts, ", "), strings.Join(parts, ", "))
}

// formatMetrics renders a benchmark's reported metrics after the known
// units, in unit order so two runs read alike.
func formatMetrics(metrics map[string]float64) string {
	units := make([]string, 0, len(metrics))
	for u := range metrics {
		units = append(units, u)
	}
	sort.Strings(units)
	var b strings.Builder
	for _, u := range units {
		fmt.Fprintf(&b, " · %s %s", strconv.FormatFloat(metrics[u], 'f', -1, 64), u)
	}
	return b.String()
}
