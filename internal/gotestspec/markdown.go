package gotestspec

import (
	"fmt"
	"io"
	"strings"
	"time"
)

// RenderMarkdown writes the spec as an outline: a header with the counts and
// where they come from, the rows that need attention, then module, package
// and suite headings over the methods, groups and behaviors as list items.
func RenderMarkdown(w io.Writer, packages []*Package, opts ...RenderOption) {
	cfg := renderConfig{color: true}
	for _, o := range opts {
		o(&cfg)
	}
	packages, _ = cfg.narrow(packages)
	bare := cfg.withoutVerdicts
	stats := CollectStats(packages)

	var doc mdDoc
	doc.block("# Behavior Specification")
	counts := strings.Join(countParts(stats), ", ")
	if bare {
		// "0 passed, 0 failed, 0 skipped" beside a document nothing ran reads
		// as a run that lost its results, rather than as a specification.
		doc.block(counts + ". Read from source; nothing was executed.")
	} else {
		verdicts := countsPhrase(stats)
		if stats.FailedPackages > 0 {
			verdicts = strings.TrimPrefix(verdicts+", "+countNoun(stats.FailedPackages, "failed package", "failed packages"), ", ")
		}
		if verdicts == "" {
			doc.block(counts + ".")
		} else {
			doc.block(counts + ": " + verdicts + ".")
		}
	}
	if line := provenanceLine(packages, &cfg); line != "" {
		doc.block(EscapeLabel(line))
	}

	if !bare {
		if rows := attention(packages, &cfg); len(rows) > 0 {
			doc.block("## Needs attention")
			var list []string
			for _, r := range rows {
				path := make([]string, len(r.Path))
				for i, p := range r.Path {
					path[i] = EscapeLabel(p)
				}
				list = append(list, "- "+verdictWord(r.Status)+" "+strings.Join(path, " › "))
				list = append(list, fenced(r.Output, "  ")...)
			}
			doc.block(list...)
		}
	}

	var inModule, other []*Package
	for _, pkg := range packages {
		if !cfg.packageShown(pkg) {
			continue
		}
		if _, out := cfg.packageName(pkg); out {
			other = append(other, pkg)
		} else {
			inModule = append(inModule, pkg)
		}
	}
	if len(inModule) > 0 {
		doc.block("## " + EscapeLabel(cfg.module))
		for _, pkg := range inModule {
			renderMarkdownPackage(&doc, pkg, &cfg)
		}
	}
	if len(other) > 0 {
		doc.block("## Other")
		for _, pkg := range other {
			renderMarkdownPackage(&doc, pkg, &cfg)
		}
	}
	_, _ = io.WriteString(w, doc.String())
}

// mdDoc is a Markdown document of blocks, one blank line between them.
type mdDoc struct{ blocks []string }

func (d *mdDoc) block(lines ...string) {
	if len(lines) > 0 {
		d.blocks = append(d.blocks, strings.Join(lines, "\n")+"\n")
	}
}

func (d *mdDoc) String() string { return strings.Join(d.blocks, "\n") }

// renderMarkdownPackage writes a package's heading with its counts, its own
// failure, its loose rows (stdlib tests, bare benchmarks) and its suites.
func renderMarkdownPackage(doc *mdDoc, pkg *Package, cfg *renderConfig) {
	bare := cfg.withoutVerdicts
	name, _ := cfg.packageName(pkg)
	heading := "### " + EscapeLabel(name)
	own := !bare && PkgFailedOnItsOwn(pkg) && !cfg.hidden[StatusFail]
	if own {
		heading += " — FAILED"
	}
	if !bare {
		if c := countsPhrase(CollectStats([]*Package{pkg})); c != "" {
			heading += " — " + c
		}
	}
	doc.block(heading)
	if own {
		// A build failure, a fixture that failed: no behavior carries it.
		doc.block(fenced(diagnostic(pkg.Output), "")...)
	}

	var suites, loose, benches []*Node
	var collect func(nodes []*Node)
	collect = func(nodes []*Node) {
		for _, n := range cfg.visible(nodes) {
			switch n.Kind {
			case KindFixture:
				collect(n.Children)
			case KindSuite:
				suites = append(suites, n)
			case KindBenchmark:
				benches = append(benches, n)
			default:
				loose = append(loose, n)
			}
		}
	}
	collect(pkg.Nodes)

	var list []string
	for _, n := range loose {
		list = markdownItem(list, n, 0, cfg)
	}
	doc.block(list...)
	doc.block(markdownBenchTable(benches, cfg)...)
	for _, s := range suites {
		renderMarkdownSuite(doc, s, cfg)
	}
}

// renderMarkdownSuite writes a suite's heading with its marks and counts, its
// own failure, its rows as items and its benchmarks as a table.
func renderMarkdownSuite(doc *mdDoc, n *Node, cfg *renderConfig) {
	bare := cfg.withoutVerdicts
	children := cfg.visible(n.Children)
	heading := "#### " + EscapeLabel(n.Display)
	if n.Focused {
		heading += " — FOCUSED"
	}
	if n.External {
		heading += " — EXTERNAL"
	}
	if bare && n.Excluded {
		// Read from source, a parked suite still lists what it declares;
		// unmarked, those behaviors read as live ones.
		heading += " — EXCLUDED"
	}
	if n.Incomplete {
		heading += " — " + incompleteMark
	}
	own := false
	if len(n.Children) == 0 {
		// A suite without behaviors has no row to carry its verdict.
		switch {
		case bare && n.Excluded:
			// Nothing ran, so nothing was skipped.
		case n.Status == StatusSkip || n.Excluded:
			heading += " — SKIPPED"
		case bare:
		case n.Status == StatusFail:
			heading += " — FAILED"
			own = true
		case noVerdict(n):
			heading += " — NOT RUN"
		}
	} else if !bare {
		own = ownFailure(n) && !cfg.hidden[StatusFail]
		if own {
			heading += " — FAILED"
		}
		if c := countsPhrase(nodeStats(n)); c != "" {
			heading += " — " + c
		}
	}
	doc.block(heading)
	if own {
		doc.block(fenced(diagnostic(n.Output), "")...)
	}

	var list []string
	var benches []*Node
	for _, c := range children {
		if c.Kind == KindBenchmark {
			benches = append(benches, c)
			continue
		}
		list = markdownItem(list, c, 0, cfg)
	}
	doc.block(list...)
	doc.block(markdownBenchTable(benches, cfg)...)
}

// incompleteMark follows a row whose children are a floor, not the whole list.
const incompleteMark = "INCOMPLETE: behaviors known only at run time"

// markdownItem appends n as a list item at depth, its failure's output fenced
// under it, then its shown children one level deeper.
func markdownItem(list []string, n *Node, depth int, cfg *renderConfig) []string {
	bare := cfg.withoutVerdicts
	indent := strings.Repeat("  ", depth)
	leaf := len(n.Children) == 0

	word := ""
	var output []string
	if !bare {
		switch {
		case n.Kind == KindFuzz || leaf:
			// One verdict per fuzz target; its seeds are evidence below it.
			word = verdictWord(n.Status)
			if n.NotRun {
				word = verdictWord(StatusNone)
			}
			if leaf && n.Status == StatusFail {
				output = filterOutput(n.Output)
			} else if !leaf && ownFailure(n) {
				output = diagnostic(n.Output)
			}
		case ownFailure(n) && !cfg.hidden[StatusFail]:
			word = verdictWord(StatusFail)
			output = diagnostic(n.Output)
		}
	}

	var label string
	switch n.Kind {
	case KindMethod, KindTest, KindFuzz:
		label = "**" + EscapeLabel(n.Display) + "**"
	default:
		if word == "" {
			label = escapeItemStart(n.Display)
		} else {
			label = EscapeLabel(n.Display)
		}
	}
	line := indent + "- "
	if word != "" {
		line += word + " "
	}
	line += label
	if n.Kind == KindFuzz {
		line += " (fuzz)"
	}
	if bare && n.Excluded {
		line += " — EXCLUDED"
	}
	if n.Incomplete {
		line += " — " + incompleteMark
	}
	if word != "" && (leaf || n.Kind == KindFuzz) && !n.NotRun && n.Status != StatusNone {
		if d := EffectiveDuration(n); d >= time.Second {
			line += " (" + formatDuration(d) + ")"
		}
	}
	list = append(list, line)
	list = append(list, fenced(output, indent+"  ")...)
	for _, c := range cfg.visible(n.Children) {
		list = markdownItem(list, c, depth+1, cfg)
	}
	return list
}

// verdictWord is a row's verdict: PASS, and in bold what needs attention;
// StatusNone is a row without a verdict.
func verdictWord(s Status) string {
	switch s {
	case StatusNone:
		return "**NOT RUN**"
	case StatusFail:
		return "**FAIL**"
	case StatusSkip:
		return "**SKIP**"
	}
	return "PASS"
}

// escapeItemStart is EscapeLabel for a label that opens an item's text, where
// a heading, quote, list marker or ordered-list number would start a block.
// Leading blanks go: four of them would open a code block.
func escapeItemStart(label string) string {
	label = strings.TrimLeft(label, " \t")
	if label == "" {
		return ""
	}
	switch label[0] {
	case '#', '>', '-', '+':
		return `\` + label[:1] + EscapeLabel(label[1:])
	}
	i := 0
	for i < len(label) && label[i] >= '0' && label[i] <= '9' {
		i++
	}
	// "1. x" and "1)" are list markers; "1.5x" is not.
	if i > 0 && i < len(label) && (label[i] == '.' || label[i] == ')') &&
		(i+1 == len(label) || label[i+1] == ' ' || label[i+1] == '\t') {
		return label[:i] + `\` + label[i:i+1] + EscapeLabel(label[i+1:])
	}
	return EscapeLabel(label)
}

// fenced is output as a text code block, each line behind indent. The fence
// is longer than any backtick run in the output, so the output cannot close
// it. Empty output is no block.
func fenced(output []string, indent string) []string {
	if len(output) == 0 {
		return nil
	}
	fence := strings.Repeat("`", max(3, longestRun(strings.Join(output, "\n"), '`')+1))
	lines := []string{indent + fence + "text"}
	for _, l := range output {
		lines = append(lines, indent+l)
	}
	return append(lines, indent+fence)
}

// longestRun is the length of the longest run of c in s.
func longestRun(s string, c byte) int {
	longest, run := 0, 0
	for i := 0; i < len(s); i++ {
		if s[i] != c {
			run = 0
			continue
		}
		run++
		longest = max(longest, run)
	}
	return longest
}

// markdownBenchTable is benchmarks as a table, numbers across rows; a
// sub-benchmark is a row named by its path.
func markdownBenchTable(nodes []*Node, cfg *renderConfig) []string {
	var rows []string
	var walk func(n *Node, name string)
	walk = func(n *Node, name string) {
		kids := cfg.visible(n.Children)
		if len(n.Children) == 0 {
			rows = append(rows, fmt.Sprintf("| %s | %s | %d | %d |",
				EscapeLabel(name), formatNs(n.NsPerOp), n.BytesPerOp, n.AllocsPerOp))
		}
		for _, c := range kids {
			walk(c, name+"/"+c.Display)
		}
	}
	for _, n := range nodes {
		walk(n, n.Display)
	}
	if len(rows) == 0 {
		return nil
	}
	return append([]string{"| Benchmark | ns/op | B/op | allocs/op |", "|-----------|-------|------|-----------|"}, rows...)
}

// markdownEscaper keeps a name in its table cell: no character of it may end
// the cell, start a row, open a code span or reach the reader as HTML. The
// backslash goes first, and the replacer makes one pass, so the escapes it
// adds are not escaped again.
var markdownEscaper = strings.NewReplacer(
	`\`, `\\`,
	"|", `\|`,
	"`", "\\`",
	"<", "&lt;",
	">", "&gt;",
	"\r\n", " ",
	"\n", " ",
	"\r", " ",
)

// labelEscaper is markdownEscaper that also keeps a label from opening a
// link, emphasis, strikethrough, a tilde fence or an entity, so the spec
// shows it as written.
var labelEscaper = strings.NewReplacer(
	`\`, `\\`,
	"|", `\|`,
	"`", "\\`",
	"*", `\*`,
	"_", `\_`,
	"~", `\~`,
	"[", `\[`,
	"]", `\]`,
	"&", "&amp;",
	"<", "&lt;",
	">", "&gt;",
	"\r\n", " ",
	"\n", " ",
	"\r", " ",
)

// EscapeMarkdown is the escape for a table cell of names gotest prints. A line
// break becomes a space: a cell is one line.
func EscapeMarkdown(s string) string { return markdownEscaper.Replace(s) }

// EscapeLabel is the escape for every label and output the Markdown spec
// writes: EscapeMarkdown, and no link, emphasis or entity either.
func EscapeLabel(s string) string { return labelEscaper.Replace(s) }
