package gotestspec

import (
	"fmt"
	"io"
	"strings"
	"time"
)

type failure struct {
	Package  string
	Display  []string
	Duration time.Duration
	Output   []string
}

func collectFailures(packages []*Package) []failure {
	var failures []failure
	for _, pkg := range packages {
		for _, node := range pkg.Nodes {
			collectFailedLeaves(pkg.Path, node, nil, &failures)
		}
	}
	return failures
}

func collectFailedLeaves(pkgPath string, n *Node, display []string, out *[]failure) {
	cur := append(append([]string(nil), display...), n.Display)

	if n.Status == StatusFail && (len(n.Children) == 0 || hasOwnDiagnostic(n) || failedOnItsOwn(n)) {
		d := make([]string, len(cur))
		copy(d, cur)
		*out = append(*out, failure{
			Package:  pkgPath,
			Display:  d,
			Duration: EffectiveDuration(n),
			Output:   n.Output,
		})
	}

	for _, child := range n.Children {
		collectFailedLeaves(pkgPath, child, cur, out)
	}
}

type packageDiagnostic struct {
	Package string
	Output  []string
}

func collectPackageDiagnostics(packages []*Package) []packageDiagnostic {
	var diags []packageDiagnostic
	for _, pkg := range packages {
		if pkg.Status != StatusFail || len(pkg.Output) == 0 {
			continue
		}
		diags = append(diags, packageDiagnostic{
			Package: pkg.Path,
			Output:  pkg.Output,
		})
	}
	return diags
}

// effectiveDuration prefers a wall clock the caller measured itself, because
// that one also covers compiling: the event stream only starts once the first
// test does. Without it, the union of what the packages occupied is the closest
// the stream can get.
func totalDuration(packages []*Package) time.Duration {
	return TotalDuration(packages)
}

func effectiveDuration(cfg renderConfig, packages []*Package) time.Duration {
	if cfg.elapsed > 0 {
		return cfg.elapsed
	}
	return totalDuration(packages)
}

func RenderSummary(w io.Writer, packages []*Package, opts ...RenderOption) {
	cfg := renderConfig{color: true}
	for _, o := range opts {
		o(&cfg)
	}
	c := ansiColors
	if !cfg.color {
		c = noColors
	}

	stats := CollectStats(packages)
	failures := collectFailures(packages)
	diags := collectPackageDiagnostics(packages)

	// A package-level failure without diagnostics still forbids the all-green
	// summary: the verdict is the package status, not the presence of output.
	if len(failures) == 0 && len(diags) == 0 && stats.FailedPackages == 0 {
		fmt.Fprintf(w, "%s%d tests passed%s (%s)\n",
			c.green, stats.Total(), c.reset,
			formatDuration(effectiveDuration(cfg, packages)))
		if cfg.coverage != nil {
			fmt.Fprintf(w, "%sCoverage: %.1f%%%s\n", c.dim, cfg.coverage.Total, c.reset)
		}
		renderBenchDeltaTable(w, cfg.benchDeltas, c)
		return
	}

	if len(failures) > 0 {
		fmt.Fprintf(w, "%s%d of %d tests failed%s\n",
			c.red, stats.Failed, stats.Total(), c.reset)

		for _, f := range failures {
			fmt.Fprintln(w)
			displayPath := strings.Join(f.Display, " / ")
			fmt.Fprintf(w, "%sFAIL%s  %s%s%s %s (%s)\n",
				c.red, c.reset,
				c.dim, f.Package, c.reset,
				displayPath,
				formatDuration(f.Duration))

			lines := filterOutput(f.Output)
			if len(lines) == 0 {
				lines = []string{noDiagnosticNote}
			}
			for _, line := range lines {
				fmt.Fprintf(w, "      %s%s%s\n", c.red, line, c.reset)
			}
		}
	} else {
		fmt.Fprintf(w, "%s%d tests passed%s — %spackage failure detected%s\n",
			c.green, stats.Total(), c.reset, c.red, c.reset)
	}

	for _, d := range diags {
		fmt.Fprintln(w)
		fmt.Fprintf(w, "%sFAIL%s  %s%s%s\n",
			c.red, c.reset, c.dim, d.Package, c.reset)
		for _, line := range filterOutput(d.Output) {
			fmt.Fprintf(w, "      %s%s%s\n", c.red, line, c.reset)
		}
	}

	fmt.Fprintln(w)
	if cfg.coverage != nil {
		fmt.Fprintf(w, "%sCoverage: %.1f%%%s\n", c.dim, cfg.coverage.Total, c.reset)
	}
	renderBenchDeltaTable(w, cfg.benchDeltas, c)
	renderSummary(w, stats, c)
}

func RenderMarkdownSummary(w io.Writer, packages []*Package, opts ...RenderOption) {
	cfg := renderConfig{}
	for _, o := range opts {
		o(&cfg)
	}

	stats := CollectStats(packages)
	failures := collectFailures(packages)
	diags := collectPackageDiagnostics(packages)

	if len(failures) == 0 && len(diags) == 0 && stats.FailedPackages == 0 {
		fmt.Fprintf(w, "### All %d tests passed (%s)\n",
			stats.Total(), formatDuration(effectiveDuration(cfg, packages)))
		if cfg.coverage != nil {
			renderMarkdownCoverage(w, cfg.coverage)
		}
		renderMarkdownBenchDeltaTable(w, cfg.benchDeltas)
		return
	}

	if len(failures) > 0 {
		fmt.Fprintf(w, "### %d of %d tests failed\n", stats.Failed, stats.Total())
	} else {
		fmt.Fprintf(w, "### %d tests passed — package failure detected\n", stats.Total())
	}

	renderMarkdownFailures(w, failures)

	for _, d := range diags {
		fmt.Fprintln(w)
		fmt.Fprintf(w, "<details>\n<summary><b>%s</b> — package-level failure</summary>\n\n",
			d.Package)
		lines := filterOutput(d.Output)
		for _, line := range lines {
			fmt.Fprintf(w, "    %s\n", line)
		}
		fmt.Fprintln(w)
		fmt.Fprintln(w, "</details>")
	}

	fmt.Fprintln(w)
	if cfg.coverage != nil {
		renderMarkdownCoverage(w, cfg.coverage)
	}

	renderMarkdownBenchDeltaTable(w, cfg.benchDeltas)

	fmt.Fprint(w, "---\n")
	var parts []string
	if stats.Suites > 0 {
		parts = append(parts, fmt.Sprintf("%d suites", stats.Suites))
	}
	if stats.Behaviors > 0 {
		parts = append(parts, fmt.Sprintf("%d behaviors", stats.Behaviors))
	}
	if stats.Tests > 0 {
		parts = append(parts, fmt.Sprintf("%d stdlib tests", stats.Tests))
	}
	if len(parts) == 0 {
		parts = append(parts, "0 suites")
	}
	trailer := fmt.Sprintf("%d passed, %d failed, %d skipped", stats.Passed, stats.Failed, stats.Skipped)
	if stats.FailedPackages > 0 {
		trailer += fmt.Sprintf(", %d failed packages", stats.FailedPackages)
	}
	fmt.Fprintf(w, "%s: %s\n", strings.Join(parts, ", "), trailer)
}

func renderMarkdownFailures(w io.Writer, failures []failure) {
	for _, f := range failures {
		displayPath := strings.Join(f.Display, " / ")
		fmt.Fprintln(w)
		fmt.Fprintf(w, "<details>\n<summary><b>%s</b> — %s (%s)</summary>\n\n",
			f.Package, displayPath, formatDuration(f.Duration))

		lines := filterOutput(f.Output)
		if len(lines) == 0 {
			lines = []string{noDiagnosticNote}
		}
		for _, line := range lines {
			fmt.Fprintf(w, "    %s\n", line)
		}
		fmt.Fprintln(w)

		fmt.Fprintln(w, "</details>")
	}
}

// RenderMarkdownBenchSummary writes the job summary of a bench run: how many
// benchmarks ran, each one's results, the delta table when a baseline was
// compared, and the gate verdict when one was set. Benchmarks are not
// tests, so the test headline never appears here.
func RenderMarkdownBenchSummary(w io.Writer, packages []*Package, opts ...RenderOption) {
	cfg := renderConfig{}
	for _, o := range opts {
		o(&cfg)
	}

	stats := CollectStats(packages)
	failures := collectFailures(packages)
	dur := formatDuration(effectiveDuration(cfg, packages))
	if len(failures) == 0 {
		fmt.Fprintf(w, "### %d benchmarks ran (%s)\n", stats.Benchmarks, dur)
	} else {
		fmt.Fprintf(w, "### %d of %d benchmarks failed (%s)\n", len(failures), stats.Benchmarks, dur)
		renderMarkdownFailures(w, failures)
	}

	for _, pkg := range packages {
		var rows []benchRow
		for _, n := range pkg.Nodes {
			collectBenchRows(n, n, &rows)
		}
		if len(rows) == 0 {
			continue
		}
		fmt.Fprintf(w, "\n**%s**\n\n", pkg.Path)
		fmt.Fprintln(w, "| Benchmark | ns/op | B/op | allocs/op |")
		fmt.Fprintln(w, "|---|---|---|---|")
		for _, r := range rows {
			if r.node.Iterations == 0 {
				fmt.Fprintf(w, "| %s | — | — | — |\n", r.key)
				continue
			}
			fmt.Fprintf(w, "| %s | %s | %d | %d |\n",
				r.key, formatNs(r.node.NsPerOp), r.node.BytesPerOp, r.node.AllocsPerOp)
		}
	}

	if cfg.benchDeltas != nil {
		fmt.Fprintln(w)
		renderMarkdownBenchDeltaTable(w, cfg.benchDeltas)
	}
	if g := cfg.benchGate; g != nil {
		if g.Breached {
			fmt.Fprintf(w, "**Bench gate breached:** %s +%.1f%% exceeds the %g%% gate\n", g.WorstKey, g.WorstPct, g.ThresholdPct)
		} else {
			fmt.Fprintf(w, "**Bench gate passed:** no regression above %g%%\n", g.ThresholdPct)
		}
	}
}

// benchRow pairs a benchmark leaf with the "Suite/Name" key the delta table
// uses, so the two tables read against each other.
type benchRow struct {
	key  string
	node *Node
}

// collectBenchRows gathers the benchmark leaves under n. top is the
// package-level ancestor whose name (minus "Benchmark") is the suite; a leaf
// that is itself top has no suite and is keyed by its own name.
func collectBenchRows(top, n *Node, out *[]benchRow) {
	if n.Kind == KindBenchmark && len(n.Children) == 0 {
		key := n.Name
		if n != top {
			key = strings.TrimPrefix(top.Name, "Benchmark") + "/" + n.Name
		}
		*out = append(*out, benchRow{key: key, node: n})
		return
	}
	for _, c := range n.Children {
		collectBenchRows(top, c, out)
	}
}

// renderMarkdownBenchDeltaTable renders deltas as a markdown table mirroring
// renderBenchDeltaTable's terminal columns. deltas is rendered as given —
// filtering significant-only vs. every row (-v) is the caller's
// responsibility (see WithBenchDeltas). A nil slice (WithBenchDeltas never
// called) no-ops; an empty-but-non-nil slice still prints the header (see
// renderBenchDeltaTable for why).
func renderMarkdownBenchDeltaTable(w io.Writer, deltas []BenchDelta) {
	if deltas == nil {
		return
	}
	fmt.Fprintln(w, "| Benchmark | old ns/op | new ns/op | Δ |")
	fmt.Fprintln(w, "|---|---|---|---|")
	for _, d := range deltas {
		sign := ""
		if d.PercentChange >= 0 {
			sign = "+"
		}
		warn := ""
		if d.Significant && d.PercentChange > 0 {
			warn = " ⚠"
		}
		fmt.Fprintf(w, "| %s | %.1f | %.1f | %s%.1f%%%s |\n",
			d.Key, d.OldNs, d.NewNs, sign, d.PercentChange, warn)
	}
	fmt.Fprintln(w)
}

func renderMarkdownCoverage(w io.Writer, report *CoverageReport) {
	fmt.Fprintf(w, "### Coverage: %.1f%%\n\n", report.Total)
	if len(report.Packages) > 1 {
		fmt.Fprintln(w, "| Package | Coverage |")
		fmt.Fprintln(w, "|---------|----------|")
		for _, pkg := range report.Packages {
			fmt.Fprintf(w, "| `%s` | %.1f%% |\n", pkg.Path, pkg.Percentage)
		}
		fmt.Fprintln(w)
	}
}
