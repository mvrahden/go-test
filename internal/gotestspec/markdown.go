package gotestspec

import (
	"fmt"
	"io"
	"strings"
)

func RenderMarkdown(w io.Writer, packages []*Package) {
	stats := CollectStats(packages)

	fmt.Fprintln(w, "# Behavior Specification")
	fmt.Fprintln(w)
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
	fmt.Fprintf(w, "%s: %d passed, %d failed, %d skipped.\n",
		strings.Join(counts, ", "), stats.Passed, stats.Failed, stats.Skipped)
	fmt.Fprintln(w)

	for _, pkg := range packages {
		for _, node := range pkg.Nodes {
			renderMarkdownNode(w, node, 2)
		}
	}
}

func renderMarkdownNode(w io.Writer, n *Node, headingLevel int) {
	switch n.Kind {
	case KindFixture:
		for _, child := range n.Children {
			renderMarkdownNode(w, child, headingLevel)
		}

	case KindSuite:
		heading := strings.Repeat("#", headingLevel)
		label := n.Display
		if n.Focused {
			label += " — FOCUSED"
		}
		if len(n.Children) == 0 {
			if n.Status == StatusSkip || n.Excluded {
				label += " — SKIPPED"
			}
			fmt.Fprintf(w, "%s %s\n\n", heading, label)
			return
		}
		fmt.Fprintf(w, "%s %s\n\n", heading, label)

		var leafChildren, benchChildren, nestedChildren []*Node
		for _, c := range n.Children {
			switch {
			case c.Kind == KindBenchmark && len(c.Children) == 0:
				benchChildren = append(benchChildren, c)
			case len(c.Children) == 0:
				leafChildren = append(leafChildren, c)
			default:
				nestedChildren = append(nestedChildren, c)
			}
		}

		if len(leafChildren) > 0 {
			fmt.Fprintln(w, "| Behavior | Status | Duration |")
			fmt.Fprintln(w, "|----------|--------|----------|")
			for _, c := range leafChildren {
				fmt.Fprintf(w, "| %s | %s | %s |\n",
					c.Display, statusText(c.Status), formatDuration(c.Duration))
			}
			fmt.Fprintln(w)
		}

		if len(benchChildren) > 0 {
			renderMarkdownBenchTable(w, benchChildren)
		}

		for _, c := range nestedChildren {
			renderMarkdownNode(w, c, headingLevel+1)
		}

	case KindMethod, KindTest:
		if len(n.Children) == 0 {
			return
		}
		heading := strings.Repeat("#", headingLevel)
		fmt.Fprintf(w, "%s %s\n\n", heading, n.Display)
		fmt.Fprintln(w, "| Behavior | Status | Duration |")
		fmt.Fprintln(w, "|----------|--------|----------|")
		renderMarkdownTable(w, n.Children, 0)
		fmt.Fprintln(w)

	case KindBenchmark:
		if len(n.Children) != 0 {
			return
		}
		// A bare top-level benchmark node with no enclosing suite.
		heading := strings.Repeat("#", headingLevel)
		fmt.Fprintf(w, "%s %s\n\n", heading, n.Display)
		renderMarkdownBenchTable(w, []*Node{n})

	default:
		if len(n.Children) == 0 {
			fmt.Fprintf(w, "- %s %s (%s)\n", statusText(n.Status), n.Display, formatDuration(n.Duration))
		}
	}
}

func renderMarkdownBenchTable(w io.Writer, nodes []*Node) {
	fmt.Fprintln(w, "| Benchmark | ns/op | B/op | allocs/op |")
	fmt.Fprintln(w, "|-----------|-------|------|-----------|")
	for _, c := range nodes {
		fmt.Fprintf(w, "| %s | %s | %d | %d |\n",
			c.Display, formatNs(c.NsPerOp), c.BytesPerOp, c.AllocsPerOp)
	}
	fmt.Fprintln(w)
}

func renderMarkdownTable(w io.Writer, nodes []*Node, depth int) {
	indent := strings.Repeat("&nbsp;&nbsp;", depth)
	for _, n := range nodes {
		if len(n.Children) > 0 {
			fmt.Fprintf(w, "| %s**%s** | | |\n", indent, n.Display)
			renderMarkdownTable(w, n.Children, depth+1)
		} else {
			fmt.Fprintf(w, "| %s%s | %s | %s |\n",
				indent, n.Display, statusText(n.Status), formatDuration(n.Duration))
		}
	}
}

func statusText(s Status) string {
	switch s {
	case StatusPass:
		return "PASS"
	case StatusFail:
		return "FAIL"
	case StatusSkip:
		return "SKIP"
	default:
		return "—"
	}
}
