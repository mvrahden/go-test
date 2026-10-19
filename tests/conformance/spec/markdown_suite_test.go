package spec_test

import (
	"strings"

	"github.com/mvrahden/go-test/internal/testkit"
	"github.com/mvrahden/go-test/pkg/gotest"
)

// SpecMarkdownTestSuite: the Markdown spec of a run and of the source, over
// labels Markdown would read as syntax, an It with a slash and X_ exclusions.
type SpecMarkdownTestSuite struct {
	live   testkit.Run
	static testkit.Run
}

const fixture = "../testdata/specmarkdown/"

// SuiteConfig: the live spec runs the fixture through the whole pipeline.
func (s *SpecMarkdownTestSuite) SuiteConfig() gotest.SuiteConfig {
	cfg := gotest.IntegrationSuiteConfig()
	cfg.Parallel = true
	return cfg
}

func (s *SpecMarkdownTestSuite) BeforeAll(t *gotest.T) {
	var err error
	s.live, err = testkit.RunCLI(t.Context(), ".", "spec", "--format=md", fixture)
	gotest.NoError(t, err)
	gotest.Equal(t, 0, s.live.Exit, "spec: %s", s.live.Stderr)
	s.static, err = testkit.RunCLI(t.Context(), ".", "spec", "--static", "--format=md", fixture)
	gotest.NoError(t, err)
	gotest.Equal(t, 0, s.static.Exit, "spec --static: %s", s.static.Stderr)
}

func (s *SpecMarkdownTestSuite) AfterAll(t *gotest.T) {}

// cells counts the cells of a table row: the pipes no backslash escapes, less
// the closing one.
func cells(row string) int {
	n := 0
	for i := 0; i < len(row); i++ {
		switch row[i] {
		case '\\':
			i++
		case '|':
			n++
		}
	}
	return n - 1
}

// tableRows is every table row of a Markdown document, header and rule included.
func tableRows(md string) []string {
	var rows []string
	for _, line := range strings.Split(md, "\n") {
		if strings.HasPrefix(line, "|") {
			rows = append(rows, line)
		}
	}
	return rows
}

func (s *SpecMarkdownTestSuite) TestLabels(t *gotest.T) {
	t.When("a label holds a pipe, a backslash, an angle bracket, a backtick or a line break", func(w *gotest.T) {
		w.It("keeps each behavior on one row of its table", func(it *gotest.T) {
			for _, doc := range []struct {
				name, md string
				width    int
				rows     []string
			}{
				{"spec", s.live.Stdout, 3, []string{
					"| **when a label holds a \\| b** | | |",
					"| &nbsp;&nbsp;keeps &lt;b&gt;bold&lt;/b&gt; and \\`code\\` as text | PASS | ",
					"| &nbsp;&nbsp;keeps a\\\\b and a line break in one row | PASS | ",
					"| row \\| &lt;i&gt;x&lt;/i&gt; | PASS | ",
				}},
				{"spec --static", s.static.Stdout, 1, []string{
					"| **when a label holds a \\| b** |\n",
					"| &nbsp;&nbsp;keeps &lt;b&gt;bold&lt;/b&gt; and \\`code\\` as text |\n",
					"| &nbsp;&nbsp;keeps a\\\\b and a line break in one row |\n",
					"| row \\| &lt;i&gt;x&lt;/i&gt; |\n",
				}},
			} {
				for _, row := range doc.rows {
					gotest.Contains(it, doc.md, row, "%s", doc.name)
				}
				for _, row := range tableRows(doc.md) {
					gotest.Equal(it, doc.width, cells(row), "%s row %q", doc.name, row)
				}
			}
		})

		w.It("renders the label as text, with no HTML tag and no code span", func(it *gotest.T) {
			for _, md := range []string{s.live.Stdout, s.static.Stdout} {
				for _, raw := range []string{"<b>", "</b>", "<i>", "</i>", "and `code`"} {
					gotest.NotContains(it, md, raw)
				}
				gotest.Contains(it, md, "keeps &lt;b&gt;bold&lt;/b&gt; and \\`code\\` as text")
			}
		})
	})
}

func (s *SpecMarkdownTestSuite) TestSlashes(t *gotest.T) {
	t.When("an It's label holds a slash", func(w *gotest.T) {
		w.It("a run's spec shows it as one It under its When", func(it *gotest.T) {
			gotest.Contains(it, s.live.Stdout, "| **when a path is read** | | |\n| &nbsp;&nbsp;reads a/b paths | PASS | ")
			gotest.NotContains(it, s.live.Stdout, "reads a**")
		})

		w.It("the spec read from source shows it as one It under its When", func(it *gotest.T) {
			gotest.Contains(it, s.static.Stdout, "| **when a path is read** |\n| &nbsp;&nbsp;reads a/b paths |\n")
			gotest.NotContains(it, s.static.Stdout, "reads a**")
		})
	})
}

func (s *SpecMarkdownTestSuite) TestExcluded(t *gotest.T) {
	t.When("the spec is read from source with X_ suites and methods", func(w *gotest.T) {
		w.It("marks each excluded suite, method and behavior EXCLUDED", func(it *gotest.T) {
			for _, want := range []string{
				"## Parked — EXCLUDED\n\n### Anything — EXCLUDED\n\n| Behavior |\n|----------|\n| **when the suite is parked** — EXCLUDED |\n| &nbsp;&nbsp;never runs — EXCLUDED |\n",
				"### Parked — EXCLUDED\n\n| Behavior |\n|----------|\n| **when the method is parked** — EXCLUDED |\n| &nbsp;&nbsp;never runs — EXCLUDED |\n",
				"## Labels\n\n### Markup\n",
			} {
				gotest.Contains(it, s.static.Stdout, want)
			}
		})
	})
}
