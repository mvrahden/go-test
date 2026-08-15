package gotestspec

import (
	"fmt"
	"io"
	"strings"
)

func RenderMarkdown(w io.Writer, packages []*Package, opts ...RenderOption) {
	cfg := renderConfig{color: true}
	for _, o := range opts {
		o(&cfg)
	}
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
	if cfg.withoutVerdicts {
		// "0 passed, 0 failed, 0 skipped" beside a document nothing ran reads
		// as a run that lost its results, rather than as a specification.
		fmt.Fprintf(w, "%s. Read from source; nothing was executed.\n", strings.Join(counts, ", "))
	} else {
		fmt.Fprintf(w, "%s: %d passed, %d failed, %d skipped.\n",
			strings.Join(counts, ", "), stats.Passed, stats.Failed, stats.Skipped)
	}
	fmt.Fprintln(w)

	for _, pkg := range packages {
		for _, node := range pkg.Nodes {
			renderMarkdownNode(w, node, 2, cfg.withoutVerdicts)
		}
	}
}

const incompleteNote = "this method declares further behaviors whose names or existence depend on runtime values"

// markdownRow renders one behavior row. A specification carries no Status or
// Duration column, because a document that prints "<1ms" beside a behavior
// nothing ran is stating a measurement that was never taken.
func markdownRow(w io.Writer, indent, display string, n *Node, bare bool) {
	if n.Incomplete {
		// A method with no behaviors to tabulate is rendered as a row of its
		// suite's table. Without the marker it reads as a behavior that simply
		// has nothing beneath it.
		display += " — _incomplete: behaviors known only at run time_"
	}
	if bare {
		fmt.Fprintf(w, "| %s%s |\n", indent, display)
		return
	}
	fmt.Fprintf(w, "| %s%s | %s | %s |\n", indent, display, statusText(n.Status), formatDuration(n.Duration))
}

func markdownHeader(w io.Writer, bare bool) {
	if bare {
		fmt.Fprintln(w, "| Behavior |")
		fmt.Fprintln(w, "|----------|")
		return
	}
	fmt.Fprintln(w, "| Behavior | Status | Duration |")
	fmt.Fprintln(w, "|----------|--------|----------|")
}

// markdownNote states what a heading cannot: that the behaviors listed under it
// are a floor rather than the whole list.
func markdownNote(w io.Writer, n *Node) {
	if n.Incomplete {
		fmt.Fprintf(w, "_Incomplete: %s._\n\n", incompleteNote)
	}
}

func renderMarkdownNode(w io.Writer, n *Node, headingLevel int, bare bool) {
	switch n.Kind {
	case KindFixture:
		for _, child := range n.Children {
			renderMarkdownNode(w, child, headingLevel, bare)
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

		var leafChildren, nestedChildren []*Node
		for _, c := range n.Children {
			if len(c.Children) == 0 {
				leafChildren = append(leafChildren, c)
			} else {
				nestedChildren = append(nestedChildren, c)
			}
		}

		if len(leafChildren) > 0 {
			markdownHeader(w, bare)
			for _, c := range leafChildren {
				markdownRow(w, "", c.Display, c, bare)
			}
			fmt.Fprintln(w)
		}

		for _, c := range nestedChildren {
			renderMarkdownNode(w, c, headingLevel+1, bare)
		}

	case KindMethod, KindTest:
		heading := strings.Repeat("#", headingLevel)
		if len(n.Children) == 0 {
			// A method whose every behavior is runtime-dependent has nothing to
			// tabulate, but dropping it would delete a declared method from the
			// specification without saying so.
			if n.Incomplete {
				fmt.Fprintf(w, "%s %s\n\n", heading, n.Display)
				markdownNote(w, n)
			}
			return
		}
		fmt.Fprintf(w, "%s %s\n\n", heading, n.Display)
		markdownNote(w, n)
		markdownHeader(w, bare)
		renderMarkdownTable(w, n.Children, 0, bare)
		fmt.Fprintln(w)

	default:
		if len(n.Children) == 0 {
			if bare {
				fmt.Fprintf(w, "- %s\n", n.Display)
				return
			}
			fmt.Fprintf(w, "- %s %s (%s)\n", statusText(n.Status), n.Display, formatDuration(n.Duration))
		}
	}
}

func renderMarkdownTable(w io.Writer, nodes []*Node, depth int, bare bool) {
	indent := strings.Repeat("&nbsp;&nbsp;", depth)
	for _, n := range nodes {
		if len(n.Children) > 0 {
			if bare {
				fmt.Fprintf(w, "| %s**%s** |\n", indent, n.Display)
			} else {
				fmt.Fprintf(w, "| %s**%s** | | |\n", indent, n.Display)
			}
			renderMarkdownTable(w, n.Children, depth+1, bare)
		} else {
			markdownRow(w, indent, n.Display, n, bare)
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
