package docref_test

import (
	"fmt"
	"strings"

	"github.com/mvrahden/go-test/internal/docref"
	"github.com/mvrahden/go-test/pkg/gotest"
)

// RewriteTestSuite: the text between a marker pair is the section's
// rendering, and nothing outside a pair changes.
type RewriteTestSuite struct{}

func (s *RewriteTestSuite) SuiteConfig() gotest.SuiteConfig {
	cfg := gotest.DefaultSuiteConfig()
	cfg.Parallel = true
	return cfg
}

func (s *RewriteTestSuite) TestRewrite(t *gotest.T) {
	sections := map[string]docref.Renderer{
		"demo/one": func(format string) (string, error) {
			if format != "table" {
				return "", fmt.Errorf(`has no format %q; formats: table`, format)
			}
			return "| a |\n", nil
		},
	}
	doc := "intro\n<!-- gotest:generated demo/one table -->\nold\ntext\n<!-- gotest:end -->\noutro\n"
	t.It("replaces what the markers hold and nothing else", func(it *gotest.T) {
		got, err := docref.Rewrite([]byte(doc), sections)
		gotest.NoError(it, err)
		gotest.Equal(it, "intro\n<!-- gotest:generated demo/one table -->\n| a |\n<!-- gotest:end -->\noutro\n", string(got))
	})
	t.It("changes nothing the second time", func(it *gotest.T) {
		once, err := docref.Rewrite([]byte(doc), sections)
		gotest.NoError(it, err)
		twice, err := docref.Rewrite(once, sections)
		gotest.NoError(it, err)
		gotest.Equal(it, string(once), string(twice))
	})
	t.It("keeps CRLF line ends outside the section", func(it *gotest.T) {
		got, err := docref.Rewrite([]byte(strings.ReplaceAll(doc, "\n", "\r\n")), sections)
		gotest.NoError(it, err)
		gotest.Contains(it, string(got), "intro\r\n")
		gotest.Contains(it, string(got), "| a |\r\n")
	})
	t.It("indents the rendering as far as its opening marker", func(it *gotest.T) {
		got, err := docref.Rewrite([]byte("<ul>\n  <!-- gotest:generated demo/one table -->\n  <!-- gotest:end -->\n</ul>\n"), sections)
		gotest.NoError(it, err)
		gotest.Equal(it, "<ul>\n  <!-- gotest:generated demo/one table -->\n  | a |\n  <!-- gotest:end -->\n</ul>\n", string(got))
	})
	type bad struct{ Desc, Doc, Want string }
	for t, r := range gotest.Each(t, []bad{
		{"an unknown section", "<!-- gotest:generated demo/two table -->\n<!-- gotest:end -->\n", `line 1: section "demo/two" is not known; known: demo/one`},
		{"an unknown format", "<!-- gotest:generated demo/one yaml -->\n<!-- gotest:end -->\n", `line 1: section "demo/one" has no format "yaml"; formats: table`},
		{"an open marker", "x\n<!-- gotest:generated demo/one table -->\n", `line 2: marker is not closed; add <!-- gotest:end -->`},
		{"an end without a start", "<!-- gotest:end -->\n", `line 1: <!-- gotest:end --> without an opening marker`},
		{"a marker inside a section", "<!-- gotest:generated demo/one table -->\n<!-- gotest:generated demo/one table -->\n<!-- gotest:end -->\n", `line 2: a marker inside the section opened at line 1; close that one first`},
	}) {
		_, err := docref.Rewrite([]byte(r.Doc), sections)
		gotest.ErrorContains(t, err, r.Want)
	}
}

func (s *RewriteTestSuite) TestSections(t *gotest.T) {
	doc := "a\n<!-- gotest:generated demo/one table -->\nx\n<!-- gotest:end -->\n  <!-- gotest:generated demo/two html -->\n  <!-- gotest:end -->\n"
	t.It("lists the marker pairs in order with the opening line", func(it *gotest.T) {
		got, err := docref.Sections([]byte(doc))
		gotest.NoError(it, err)
		gotest.Equal(it, []docref.Marker{{Section: "demo/one", Format: "table", Line: 2}, {Section: "demo/two", Format: "html", Line: 5}}, got)
	})
	t.It("reads a marker named in running text as text", func(it *gotest.T) {
		got, err := docref.Sections([]byte("The pair `<!-- gotest:generated a b -->` opens a section.\n"))
		gotest.NoError(it, err)
		gotest.Empty(it, got)
	})
}
