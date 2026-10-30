package gotestspec_test

import (
	"bytes"
	"strings"
	"time"

	"github.com/mvrahden/go-test/internal/gotestspec"
	"github.com/mvrahden/go-test/pkg/gotest"
)

// MarkdownSafetyTestSuite covers what a label can do to the Markdown spec: a
// label is text, so no character in it may end a cell, start a row, open a
// code span or reach a reader as HTML.
type MarkdownSafetyTestSuite struct{}

func (s *MarkdownSafetyTestSuite) SuiteConfig() gotest.SuiteConfig {
	cfg := gotest.DefaultSuiteConfig()
	cfg.Parallel = true
	return cfg
}

type escapeCase struct {
	Desc string
	in   string
	want string
}

func (s *MarkdownSafetyTestSuite) TestEscapeMarkdown(t *gotest.T) {
	t.When("a label holds one character Markdown reads as syntax", func(w *gotest.T) {
		for it, c := range gotest.Each(w, []escapeCase{
			{Desc: "pipe", in: "a|b", want: `a\|b`},
			{Desc: "backslash", in: `a\b`, want: `a\\b`},
			{Desc: "less than", in: "a<b", want: "a&lt;b"},
			{Desc: "greater than", in: "a>b", want: "a&gt;b"},
			{Desc: "backtick", in: "a`b", want: "a\\`b"},
			{Desc: "line feed", in: "a\nb", want: "a b"},
			{Desc: "carriage return", in: "a\rb", want: "a b"},
			{Desc: "CRLF", in: "a\r\nb", want: "a b"},
			{Desc: "ampersand", in: "a&#124;b", want: "a&amp;#124;b"},
			{Desc: "asterisk", in: "a*b", want: `a\*b`},
			{Desc: "underscore", in: "a_b", want: `a\_b`},
			{Desc: "brackets", in: "[l](http://x)", want: `\[l\](http://x)`},
			{Desc: "plain text", in: "returns ok", want: "returns ok"},
		}) {
			gotest.Equal(it, c.want, gotestspec.EscapeLabel(c.in))
		}
	})

	t.When("a label mixes them", func(w *gotest.T) {
		w.It("escapes each one once, the backslash before the rest", func(it *gotest.T) {
			gotest.Equal(it, "\\|\\\\ &lt;b&gt;\\`x\\` \\\\\\| &amp;lt; \\\\\\* \\[\\_\\]",
				gotestspec.EscapeLabel("|\\\n<b>`x`\r\n\\| &lt; \\* [_]"))
		})
	})
}

// hostileTree holds every place the Markdown renderer writes a label: suite
// and method headings, When rows, It rows, Each rows, a childless method row,
// a benchmark row and a bare list item.
func hostileTree() []*gotestspec.Package {
	return []*gotestspec.Package{{
		Path: "example.com/pkg",
		Nodes: []*gotestspec.Node{
			{
				Kind:    gotestspec.KindSuite,
				Display: "S<i>|",
				Children: []*gotestspec.Node{
					{
						Kind:    gotestspec.KindMethod,
						Display: "M`x",
						Children: []*gotestspec.Node{{
							Kind:    gotestspec.KindBlock,
							Display: "when a | b*",
							Children: []*gotestspec.Node{
								{Kind: gotestspec.KindBlock, Display: "keeps <b>bold</b>", Status: gotestspec.StatusPass},
								{Kind: gotestspec.KindBlock, Display: "row\nline | `c`", Status: gotestspec.StatusPass},
								{Kind: gotestspec.KindBlock, Display: "links [l](http://x) &#124; _x_", Status: gotestspec.StatusPass},
							},
						}},
					},
					{Kind: gotestspec.KindMethod, Display: "leaf|<x>", Status: gotestspec.StatusPass},
					{Kind: gotestspec.KindBenchmark, Display: "bench|<y>"},
				},
			},
			{Kind: gotestspec.KindBlock, Display: "item <z>\nnext", Status: gotestspec.StatusPass},
		},
	}}
}

// cellCount counts the cells of a table row: the pipes no backslash escapes,
// less the closing one.
func cellCount(row string) int {
	n := 0
	for i := 0; i < len(row); i++ {
		if row[i] == '\\' {
			i++
			continue
		}
		if row[i] == '|' {
			n++
		}
	}
	return n - 1
}

func renderMarkdownOf(packages []*gotestspec.Package, opts ...gotestspec.RenderOption) string {
	var buf bytes.Buffer
	gotestspec.RenderMarkdown(&buf, packages, opts...)
	return buf.String()
}

func (s *MarkdownSafetyTestSuite) TestRenderMarkdown_HostileLabels(t *gotest.T) {
	for _, mode := range []struct {
		name  string
		opts  []gotestspec.RenderOption
		cells int
	}{
		{"a run", nil, 3},
		{"the source", []gotestspec.RenderOption{gotestspec.WithoutVerdicts()}, 1},
	} {
		out := renderMarkdownOf(hostileTree(), mode.opts...)
		t.When("the markdown spec of "+mode.name+" renders labels that hold Markdown syntax", func(w *gotest.T) {
			w.It("keeps every behavior row on one line with its own cells", func(it *gotest.T) {
				rows := 0
				for _, line := range strings.Split(out, "\n") {
					if !strings.HasPrefix(line, "| ") || strings.HasPrefix(line, "| Benchmark |") {
						continue
					}
					rows++
					if strings.Contains(line, "ns/op") || strings.Contains(line, "bench") {
						gotest.Equal(it, 4, cellCount(line), "row %q", line)
						continue
					}
					gotest.Equal(it, mode.cells, cellCount(line), "row %q", line)
				}
				gotest.Equal(it, 8, rows, "rows:\n%s", out)
				gotest.NotContains(it, out, "\nline")
				gotest.NotContains(it, out, "\nnext")
			})

			w.It("reaches the reader with no HTML tag, entity, link, emphasis or code span", func(it *gotest.T) {
				for _, tag := range []string{"<b>", "</b>", "<i>", "<x>", "<y>", "<z>"} {
					gotest.NotContains(it, out, tag)
				}
				gotest.NotContains(it, out, " `c`")
				gotest.NotContains(it, out, "M`x")
				gotest.NotContains(it, out, "[l](")
				gotest.NotContains(it, out, " &#124;")
				gotest.NotContains(it, out, " _x_")
			})

			w.It("shows each label as written", func(it *gotest.T) {
				for _, want := range []string{
					"## S&lt;i&gt;\\|",
					"### M\\`x",
					"**when a \\| b\\***",
					"links \\[l\\](http://x) &amp;#124; \\_x\\_",
					"keeps &lt;b&gt;bold&lt;/b&gt;",
					"row line \\| \\`c\\`",
					"leaf\\|&lt;x&gt;",
					"bench\\|&lt;y&gt;",
					"item &lt;z&gt; next",
				} {
					gotest.Contains(it, out, want)
				}
			})
		})
	}
}

func (s *MarkdownSafetyTestSuite) TestRenderMarkdown_OutputHoldsAFence(t *gotest.T) {
	out := renderMarkdownOf([]*gotestspec.Package{{
		Path:   "example.com/pkg",
		Status: gotestspec.StatusFail,
		Output: []string{"panic: boom\n", "```\n", "## injected\n"},
	}})
	t.When("a failure's output holds a code fence", func(w *gotest.T) {
		w.It("opens the block with a longer fence, so the output stays inside it", func(it *gotest.T) {
			gotest.Contains(it, out, "````\npanic: boom\n```\n## injected\n````\n")
		})
	})
}

const slashStream = `{"Action":"run","Package":"example.com/pkg","Test":"TestPathsTestSuite"}
{"Action":"run","Package":"example.com/pkg","Test":"TestPathsTestSuite/TestRead"}
{"Action":"run","Package":"example.com/pkg","Test":"TestPathsTestSuite/TestRead/a_path_is_read"}
{"Action":"run","Package":"example.com/pkg","Test":"TestPathsTestSuite/TestRead/a_path_is_read/reads_a/b_paths"}
{"Action":"pass","Package":"example.com/pkg","Test":"TestPathsTestSuite/TestRead/a_path_is_read/reads_a/b_paths","Elapsed":0.001}
{"Action":"run","Package":"example.com/pkg","Test":"TestPathsTestSuite/TestRead/a_path_is_read/nested"}
{"Action":"run","Package":"example.com/pkg","Test":"TestPathsTestSuite/TestRead/a_path_is_read/nested/x"}
{"Action":"pass","Package":"example.com/pkg","Test":"TestPathsTestSuite/TestRead/a_path_is_read/nested/x","Elapsed":0.001}
{"Action":"pass","Package":"example.com/pkg","Test":"TestPathsTestSuite/TestRead/a_path_is_read/nested","Elapsed":0.001}
{"Action":"pass","Package":"example.com/pkg","Test":"TestPathsTestSuite/TestRead/a_path_is_read","Elapsed":0.002}
{"Action":"run","Package":"example.com/pkg","Test":"TestPathsTestSuite/TestRead/x/y_group"}
{"Action":"run","Package":"example.com/pkg","Test":"TestPathsTestSuite/TestRead/x/y_group/works"}
{"Action":"pass","Package":"example.com/pkg","Test":"TestPathsTestSuite/TestRead/x/y_group/works","Elapsed":0.001}
{"Action":"pass","Package":"example.com/pkg","Test":"TestPathsTestSuite/TestRead/x/y_group","Elapsed":0.001}
{"Action":"pass","Package":"example.com/pkg","Test":"TestPathsTestSuite/TestRead","Elapsed":0.002}
{"Action":"pass","Package":"example.com/pkg","Test":"TestPathsTestSuite","Elapsed":0.003}
{"Action":"pass","Package":"example.com/pkg","Elapsed":0.004}
`

func slashTree(t *gotest.T, opts ...gotestspec.BuildOption) []*gotestspec.Package {
	events, err := gotestspec.ParseEvents(strings.NewReader(slashStream))
	gotest.NoError(t, err)
	return gotestspec.BuildTree(events, opts...)
}

func (s *MarkdownSafetyTestSuite) TestBuildTree_ASlashInAnIt(t *gotest.T) {
	decls := gotestspec.DeclarationIndex{"example.com/pkg": {
		"TestPathsTestSuite/TestRead/a_path_is_read":                 {Label: "a path is read", Vocab: gotestspec.VocabWhen},
		"TestPathsTestSuite/TestRead/a_path_is_read/reads_a/b_paths": {Label: "reads a/b paths", Vocab: gotestspec.VocabIt},
	}}
	for _, c := range []struct {
		name string
		opts []gotestspec.BuildOption
	}{
		{"with its declaration", []gotestspec.BuildOption{gotestspec.WithDeclarations(decls)}},
		{"from the stream alone", nil},
	} {
		pkgs := slashTree(t, c.opts...)
		gotest.Len(t, pkgs, 1)
		when := pkgs[0].Nodes[0].Children[0].Children[0]
		t.When("a run reports an It whose label holds a slash, "+c.name, func(w *gotest.T) {
			w.It("is one It under its When, not a level of its own", func(it *gotest.T) {
				gotest.Equal(it, "a_path_is_read", when.Name)
				gotest.Len(it, when.Children, 2)
				gotest.Equal(it, "reads a/b paths", when.Children[0].Display)
				gotest.Equal(it, "reads_a/b_paths", when.Children[0].Name)
				gotest.Empty(it, when.Children[0].Children)
				gotest.Equal(it, gotestspec.StatusPass, when.Children[0].Status)
			})

			w.It("still nests a subtest that ran under one", func(it *gotest.T) {
				gotest.Equal(it, "nested", when.Children[1].Name)
				gotest.Len(it, when.Children[1].Children, 1)
				gotest.Equal(it, "x", when.Children[1].Children[0].Name)
			})

			w.It("keeps a When whose label holds a slash one level, with its It under it", func(it *gotest.T) {
				method := pkgs[0].Nodes[0].Children[0]
				gotest.Len(it, method.Children, 2)
				group := method.Children[1]
				gotest.Equal(it, "x/y_group", group.Name)
				gotest.Len(it, group.Children, 1)
				gotest.Equal(it, "works", group.Children[0].Name)
			})

			w.It("renders no level for the text before the slash", func(it *gotest.T) {
				out := renderMarkdownOf(pkgs)
				gotest.Contains(it, out, "| &nbsp;&nbsp;reads a/b paths | PASS |")
				gotest.NotContains(it, out, "reads a**")
				gotest.NotContains(it, out, "&nbsp;&nbsp;&nbsp;&nbsp;b paths")
			})
		})
	}
}

// siblingSlashStream runs an It "a" to a failure, then an It "a/x" beside it.
const siblingSlashStream = `{"Action":"run","Package":"example.com/pkg","Test":"TestPathsTestSuite"}
{"Action":"run","Package":"example.com/pkg","Test":"TestPathsTestSuite/TestRead"}
{"Action":"run","Package":"example.com/pkg","Test":"TestPathsTestSuite/TestRead/w"}
{"Action":"run","Package":"example.com/pkg","Test":"TestPathsTestSuite/TestRead/w/a"}
{"Action":"fail","Package":"example.com/pkg","Test":"TestPathsTestSuite/TestRead/w/a","Elapsed":0.001}
{"Action":"run","Package":"example.com/pkg","Test":"TestPathsTestSuite/TestRead/w/a/x"}
{"Action":"pass","Package":"example.com/pkg","Test":"TestPathsTestSuite/TestRead/w/a/x","Elapsed":0.001}
{"Action":"fail","Package":"example.com/pkg","Test":"TestPathsTestSuite/TestRead/w","Elapsed":0.002}
{"Action":"fail","Package":"example.com/pkg","Test":"TestPathsTestSuite/TestRead","Elapsed":0.002}
{"Action":"fail","Package":"example.com/pkg","Test":"TestPathsTestSuite","Elapsed":0.003}
{"Action":"fail","Package":"example.com/pkg","Elapsed":0.004}
`

func (s *MarkdownSafetyTestSuite) TestBuildTree_ASlashAfterAFinishedSibling(t *gotest.T) {
	events, err := gotestspec.ParseEvents(strings.NewReader(siblingSlashStream))
	gotest.NoError(t, err)
	pkgs := gotestspec.BuildTree(events)
	gotest.Len(t, pkgs, 1)
	when := pkgs[0].Nodes[0].Children[0].Children[0]
	t.When("an It whose label holds a slash runs after the It its label begins with finished", func(w *gotest.T) {
		w.It("is a sibling of that It, which keeps its own verdict", func(it *gotest.T) {
			gotest.Len(it, when.Children, 2)
			gotest.Equal(it, "a", when.Children[0].Name)
			gotest.Empty(it, when.Children[0].Children)
			gotest.Equal(it, gotestspec.StatusFail, when.Children[0].Status)
			gotest.Equal(it, "a/x", when.Children[1].Name)
			gotest.Equal(it, gotestspec.StatusPass, when.Children[1].Status)
		})

		w.It("counts both of them", func(it *gotest.T) {
			stats := gotestspec.CollectStats(pkgs)
			gotest.Equal(it, 1, stats.Passed)
			gotest.Equal(it, 1, stats.Failed)
		})
	})
}

// groupFailurePackages is a When whose body failed after its only It passed.
func groupFailurePackages(output ...string) []*gotestspec.Package {
	return []*gotestspec.Package{{
		Path:   "example.com/pkg",
		Status: gotestspec.StatusFail,
		Nodes: []*gotestspec.Node{{
			Kind: gotestspec.KindSuite, Display: "Boot", Status: gotestspec.StatusFail,
			Children: []*gotestspec.Node{{
				Kind: gotestspec.KindMethod, Display: "Start", Status: gotestspec.StatusFail,
				Children: []*gotestspec.Node{{
					Kind: gotestspec.KindBlock, Display: "when the disk | is full", Status: gotestspec.StatusFail,
					Duration: 2 * time.Millisecond,
					Output:   output,
					Children: []*gotestspec.Node{
						{Kind: gotestspec.KindBlock, Display: "starts", Status: gotestspec.StatusPass, Duration: time.Millisecond},
					},
				}},
			}},
		}},
	}}
}

func (s *MarkdownSafetyTestSuite) TestRenderMarkdown_AGroupFailsOnItsOwn(t *gotest.T) {
	out := renderMarkdownOf(groupFailurePackages(
		"=== RUN   TestBootTestSuite/TestStart/the_disk_|_is_full\n",
		"    boot_test.go:9: no space <left>\n",
		"    boot_test.go:10: retry later\n",
		"--- FAIL: TestBootTestSuite/TestStart/the_disk_|_is_full (0.00s)\n",
	))
	bare := renderMarkdownOf(groupFailurePackages())
	t.When("a group fails with every behavior under it passing", func(w *gotest.T) {
		w.It("shows the failure and what it wrote on the group's row", func(it *gotest.T) {
			gotest.Contains(it, out, "| **when the disk \\| is full** — boot\\_test.go:9: no space &lt;left&gt; boot\\_test.go:10: retry later | FAIL | 2ms |\n")
			gotest.Contains(it, out, "| &nbsp;&nbsp;starts | PASS | ")
		})

		w.It("still shows the failure when the group wrote nothing", func(it *gotest.T) {
			gotest.Contains(it, bare, "| **when the disk \\| is full** — test failed without diagnostic output (bare Fail?) | FAIL | 2ms |\n")
		})
	})
}

func excludedTree() []*gotestspec.Package {
	return []*gotestspec.Package{{
		Path: "example.com/pkg",
		Nodes: []*gotestspec.Node{
			{
				Kind: gotestspec.KindSuite, Display: "Labels",
				Children: []*gotestspec.Node{
					{
						Kind: gotestspec.KindMethod, Display: "Parked", Excluded: true,
						Children: []*gotestspec.Node{{Kind: gotestspec.KindBlock, Display: "never runs", Excluded: true}},
					},
					{
						Kind: gotestspec.KindMethod, Display: "Live",
						Children: []*gotestspec.Node{{Kind: gotestspec.KindBlock, Display: "runs"}},
					},
					{Kind: gotestspec.KindMethod, Display: "ParkedLeaf", Excluded: true},
				},
			},
			{
				Kind: gotestspec.KindSuite, Display: "Parked", Excluded: true,
				Children: []*gotestspec.Node{{
					Kind: gotestspec.KindMethod, Display: "Anything", Excluded: true,
					Children: []*gotestspec.Node{{Kind: gotestspec.KindBlock, Display: "never runs", Excluded: true}},
				}},
			},
			{Kind: gotestspec.KindSuite, Display: "Empty", Excluded: true},
		},
	}}
}

func (s *MarkdownSafetyTestSuite) TestRenderMarkdown_ExcludedFromSource(t *gotest.T) {
	out := renderMarkdownOf(excludedTree(), gotestspec.WithoutVerdicts())
	t.When("the markdown spec read from source holds X_ suites and methods", func(w *gotest.T) {
		w.It("marks each excluded suite, method and behavior EXCLUDED", func(it *gotest.T) {
			for _, want := range []string{
				"## Parked — EXCLUDED\n",
				"### Anything — EXCLUDED\n",
				"### Parked — EXCLUDED\n",
				"| never runs — EXCLUDED |",
				"| ParkedLeaf — EXCLUDED |",
				"## Empty — EXCLUDED\n",
			} {
				gotest.Contains(it, out, want)
			}
		})

		w.It("leaves what is not excluded unmarked", func(it *gotest.T) {
			gotest.Contains(it, out, "## Labels\n")
			gotest.Contains(it, out, "### Live\n")
			gotest.Contains(it, out, "| runs |")
		})
	})
}
