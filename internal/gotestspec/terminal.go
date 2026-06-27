package gotestspec

import (
	"fmt"
	"io"
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
	color    bool
	coverage *CoverageReport
	elapsed  time.Duration
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
	}

	fmt.Fprintln(w)
	stats := CollectStats(packages)
	renderSummary(w, stats, c)
}

func renderNode(w io.Writer, n *Node, depth int, c *colors) {
	indent := strings.Repeat("  ", depth)
	isLeaf := len(n.Children) == 0

	if isLeaf {
		icon, clr := statusIcon(n.Status, c)
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
	if n.Kind == KindSuite || n.Kind == KindFixture || n.Kind == KindMethod || n.Kind == KindTest {
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

	fmt.Fprintf(w, "%s%s%s\n", indent, label, suffix)

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

func renderSummary(w io.Writer, stats Stats, c colors) { //nolint:gocritic
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

	fmt.Fprintf(w, "%s: %s\n", strings.Join(counts, ", "), strings.Join(parts, ", "))
}
