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

	if len(n.Children) == 0 {
		if n.Status == StatusFail {
			d := make([]string, len(cur))
			copy(d, cur)
			*out = append(*out, failure{
				Package:  pkgPath,
				Display:  d,
				Duration: n.Duration,
				Output:   n.Output,
			})
		}
		return
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

func totalDuration(packages []*Package) time.Duration {
	var d time.Duration
	for _, pkg := range packages {
		d += pkg.Duration
	}
	return d
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

	if len(failures) == 0 && len(diags) == 0 {
		fmt.Fprintf(w, "%s%d tests passed%s (%s)\n",
			c.green, stats.Total(), c.reset,
			formatDuration(effectiveDuration(cfg, packages)))
		if cfg.coverage != nil {
			fmt.Fprintf(w, "%sCoverage: %.1f%%%s\n", c.dim, cfg.coverage.Total, c.reset)
		}
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

			for _, line := range filterOutput(f.Output) {
				fmt.Fprintf(w, "      %s%s%s\n", c.red, line, c.reset)
			}
		}
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

	if len(failures) == 0 && len(diags) == 0 {
		fmt.Fprintf(w, "### All %d tests passed (%s)\n",
			stats.Total(), formatDuration(effectiveDuration(cfg, packages)))
		if cfg.coverage != nil {
			renderMarkdownCoverage(w, cfg.coverage)
		}
		return
	}

	if len(failures) > 0 {
		fmt.Fprintf(w, "### %d of %d tests failed\n", stats.Failed, stats.Total())
	} else {
		fmt.Fprintf(w, "### %d tests passed — package failure detected\n", stats.Total())
	}

	for _, f := range failures {
		displayPath := strings.Join(f.Display, " / ")
		fmt.Fprintln(w)
		fmt.Fprintf(w, "<details>\n<summary><b>%s</b> — %s (%s)</summary>\n\n",
			f.Package, displayPath, formatDuration(f.Duration))

		lines := filterOutput(f.Output)
		if len(lines) > 0 {
			for _, line := range lines {
				fmt.Fprintf(w, "    %s\n", line)
			}
			fmt.Fprintln(w)
		}

		fmt.Fprintln(w, "</details>")
	}

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
	fmt.Fprintf(w, "%s: %d passed, %d failed, %d skipped\n",
		strings.Join(parts, ", "), stats.Passed, stats.Failed, stats.Skipped)
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
