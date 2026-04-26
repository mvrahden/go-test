package gotestspec

import (
	"fmt"
	"io"
	"strings"
	"time"
)

const (
	colorReset  = "\033[0m"
	colorRed    = "\033[31m"
	colorGreen  = "\033[32m"
	colorYellow = "\033[33m"
	colorBold   = "\033[1m"
	colorDim    = "\033[2m"
)

func RenderTerminal(w io.Writer, packages []*Package) {
	multiPkg := len(packages) > 1

	for i, pkg := range packages {
		if multiPkg {
			if i > 0 {
				fmt.Fprintln(w)
			}
			fmt.Fprintf(w, "%s=== %s ===%s\n", colorDim, pkg.Path, colorReset)
			fmt.Fprintln(w)
		}
		for _, node := range pkg.Nodes {
			renderNode(w, node, 0)
		}
	}

	fmt.Fprintln(w)
	stats := CollectStats(packages)
	renderSummary(w, stats)
}

func renderNode(w io.Writer, n *Node, depth int) {
	indent := strings.Repeat("  ", depth)
	isLeaf := len(n.Children) == 0

	if isLeaf {
		icon, color := statusIcon(n.Status)
		dur := formatDuration(n.Duration)

		suffix := ""
		if n.Excluded || n.Status == StatusSkip {
			suffix = " — SKIPPED"
		}

		fmt.Fprintf(w, "%s%s%s%s %s%s %s(%s)%s\n",
			indent, color, icon, colorReset,
			n.Display, suffix,
			colorDim, dur, colorReset)

		if n.Status == StatusFail {
			renderErrorOutput(w, n.Output, depth+2)
		}
		return
	}

	label := n.Display
	if n.Kind == KindSuite || n.Kind == KindFixture || n.Kind == KindMethod {
		label = colorBold + label + colorReset
	}

	suffix := ""
	if n.Focused {
		suffix = fmt.Sprintf(" %s— FOCUSED%s", colorYellow, colorReset)
	} else if n.Excluded {
		suffix = fmt.Sprintf(" %s— SKIPPED%s", colorYellow, colorReset)
	}

	if depth > 0 || n.Kind != KindFixture {
		fmt.Fprintf(w, "%s%s%s\n", indent, label, suffix)
	} else {
		fmt.Fprintf(w, "%s%s%s\n", indent, label, suffix)
	}

	for _, child := range n.Children {
		renderNode(w, child, depth+1)
	}
}

func statusIcon(s Status) (string, string) {
	switch s {
	case StatusPass:
		return "✓", colorGreen
	case StatusFail:
		return "✗", colorRed
	case StatusSkip:
		return "~", colorYellow
	default:
		return "?", colorDim
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

func renderErrorOutput(w io.Writer, output []string, depth int) {
	indent := strings.Repeat("  ", depth)
	for _, line := range output {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" {
			continue
		}
		if strings.HasPrefix(trimmed, "=== ") || strings.HasPrefix(trimmed, "--- ") {
			continue
		}
		fmt.Fprintf(w, "%s%s%s%s\n", indent, colorRed, trimmed, colorReset)
	}
}

func renderSummary(w io.Writer, stats Stats) {
	total := stats.Total()

	var parts []string
	if stats.Passed > 0 {
		parts = append(parts, fmt.Sprintf("%s%d passed%s", colorGreen, stats.Passed, colorReset))
	}
	if stats.Failed > 0 {
		parts = append(parts, fmt.Sprintf("%s%d failed%s", colorRed, stats.Failed, colorReset))
	}
	if stats.Skipped > 0 {
		parts = append(parts, fmt.Sprintf("%s%d skipped%s", colorYellow, stats.Skipped, colorReset))
	}

	fmt.Fprintf(w, "%d suites, %d behaviors: %s\n", stats.Suites, total, strings.Join(parts, ", "))
}
