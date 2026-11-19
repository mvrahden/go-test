package docref_test

import (
	"path/filepath"

	"github.com/mvrahden/go-test/internal/docref"
	"github.com/mvrahden/go-test/pkg/gotest"
)

// AssertionsTestSuite reads the real pkg/gotest: every assertion it exports
// is in one group, and a group renders as the reference writes it.
type AssertionsTestSuite struct{}

func (s *AssertionsTestSuite) SuiteConfig() gotest.SuiteConfig {
	cfg := gotest.DefaultSuiteConfig()
	cfg.Parallel = true
	return cfg
}

func (s *AssertionsTestSuite) TestAssertions(t *gotest.T) {
	as, err := docref.Assertions(filepath.Join("..", "..", "pkg", "gotest"))
	gotest.NoError(t, err)
	byName := map[string]docref.Assertion{}
	for _, a := range as {
		byName[a.Name] = a
	}
	t.It("renders a generic signature as the reference writes it", func(it *gotest.T) {
		gotest.Equal(it, "gotest.Equal[V any](t, expected, actual V)", byName["Equal"].Signature)
		gotest.Equal(it, "equality", byName["Equal"].Group)
	})
	t.It("keeps a result", func(it *gotest.T) {
		gotest.Equal(it, "gotest.Panics(t, f func()) any", byName["Panics"].Signature+" "+byName["Panics"].Result)
	})
	t.It("drops the trailing message arguments", func(it *gotest.T) {
		gotest.NotContains(it, byName["True"].Signature, "msgAndArgs")
	})
	t.It("keeps trailing arguments that are not a message", func(it *gotest.T) {
		gotest.Equal(it, "gotest.MatchSnapshot(t, value any, name ...string)", byName["MatchSnapshot"].Signature)
	})
	t.It("names the package's own types as a caller writes them", func(it *gotest.T) {
		gotest.Equal(it, "gotest.Eventually(t, waitFor, tick time.Duration, fn func(poll *gotest.R))", byName["Eventually"].Signature)
	})
	t.It("takes the first sentence of the doc comment, without the name", func(it *gotest.T) {
		gotest.Equal(it, "asserts that value is true.", byName["True"].Doc)
	})
	t.It("puts every assertion in exactly one group", func(it *gotest.T) {
		for _, a := range as {
			gotest.NotEmpty(it, a.Group, "%s is in no group of internal/docref; add it to one", a.Name)
		}
	})
	t.It("names no assertion the package does not export", func(it *gotest.T) {
		gotest.NoError(it, docref.ExportCheckGroups(as))
	})
	t.It("renders a group as a fenced block of signatures", func(it *gotest.T) {
		got, err := docref.AssertionSections(as)["assertions/boolean"]("go")
		gotest.NoError(it, err)
		gotest.Regexp(it, "^```go\ngotest\\.True\\(t, value bool\\) +// .+\ngotest\\.False\\(t, value bool\\) +// .+\n```\n$", got)
	})
	t.It("renders a group as a Markdown table", func(it *gotest.T) {
		got, err := docref.AssertionSections(as)["assertions/boolean"]("table")
		gotest.NoError(it, err)
		gotest.Regexp(it, "^\\| Assertion \\| Checks \\|\n\\|---\\|---\\|\n\\| `gotest\\.True\\(t, value bool\\)` \\| ", got)
	})
	t.It("renders a group as HTML table rows", func(it *gotest.T) {
		got, err := docref.AssertionSections(as)["assertions/boolean"]("html")
		gotest.NoError(it, err)
		gotest.Regexp(it, "^<tr><td><code>gotest\\.True\\(t, value bool\\)</code></td><td>", got)
	})
	t.It("renders every group in the section of all of them", func(it *gotest.T) {
		got, err := docref.AssertionSections(as)["assertions/all"]("table")
		gotest.NoError(it, err)
		gotest.Contains(it, got, "`gotest.Fail(t)`")
		gotest.Contains(it, got, "`gotest.ErrorAs[E error](t, err error) E`")
	})
	t.It("keeps the prose of a table cell from reading as a tag or emphasis", func(it *gotest.T) {
		got, err := docref.AssertionSections(as)["assertions/snapshots"]("table")
		gotest.NoError(it, err)
		gotest.Contains(it, got, `testdata/\_\_snapshots\_\_/&lt;TestName>.snap`)
	})
	t.It("refuses a format it has not", func(it *gotest.T) {
		_, err := docref.AssertionSections(as)["assertions/boolean"]("yaml")
		gotest.ErrorContains(it, err, `has no format "yaml"; formats: go, html, table`)
	})
}
