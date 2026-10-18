package speccatalog_test

import (
	"strings"

	"github.com/mvrahden/go-test/internal/speccatalog"
	"github.com/mvrahden/go-test/pkg/gotest"
)

// LinksTestSuite: guides link a requirement in specs/ by its GitHub anchor,
// and a link is read with its file and line.
type LinksTestSuite struct{}

func (s *LinksTestSuite) SuiteConfig() gotest.SuiteConfig {
	cfg := gotest.DefaultSuiteConfig()
	cfg.Parallel = true
	return cfg
}

func (s *LinksTestSuite) TestSlug(t *gotest.T) {
	type entry struct {
		Desc    string
		Heading string
		Slug    string
	}
	for t, e := range gotest.Each(t, []entry{
		{Desc: "a requirement with a flag", Heading: "Requirement: The go test -json stream", Slug: "requirement-the-go-test--json-stream"},
		{Desc: "a one-word requirement", Heading: "Requirement: Version", Slug: "requirement-version"},
		{Desc: "an apostrophe and inline code", Heading: "The run's `-count` flag", Slug: "the-runs--count-flag"},
		{Desc: "punctuation and capitals", Heading: "Gate: 4 samples, ±20%!", Slug: "gate-4-samples-20"},
		{Desc: "an underscore", Heading: "X_ and F_ prefixes", Slug: "x_-and-f_-prefixes"},
	}) {
		gotest.Equal(t, e.Slug, speccatalog.Slug(e.Heading))
	}
}

const anchorDoc = "# Title\n\n## Requirements\n\n### Requirement: Version\n\n```\n# Not a heading\n```\n\n### Requirement: Version\n"

func (s *LinksTestSuite) TestAnchors(t *gotest.T) {
	anchors := speccatalog.Anchors([]byte(anchorDoc))
	t.When("a document holds headings", func(w *gotest.T) {
		w.It("has an anchor per heading outside fences, numbering repeats", func(it *gotest.T) {
			gotest.Equal(it, map[string]bool{"title": true, "requirements": true, "requirement-version": true, "requirement-version-1": true}, anchors)
		})
	})
}

const linkDoc = "See [the version requirement](../../specs/cli/spec.md#requirement-version).\n" +
	"\n" +
	"```\n[fenced](../../specs/x.md)\n```\n" +
	"[ref]: ../../specs/bench/spec.md\n" +
	"Online: https://github.com/mvrahden/go-test/blob/main/specs/cli/spec.md#requirement-version and [web](https://example.com).\n" +
	"[other](fixtures.md#cli) and [top](#top).\n"

func (s *LinksTestSuite) TestSpecLinks(t *gotest.T) {
	links := speccatalog.SpecLinks("docs/design/spec.md", []byte(linkDoc))
	t.When("a guide links into specs/ relatively, by reference and by GitHub URL", func(w *gotest.T) {
		w.It("reads each with its line, the spec file and the anchor, and skips fences and other targets", func(it *gotest.T) {
			gotest.Equal(it, []speccatalog.SpecLink{
				{File: "docs/design/spec.md", Line: 1, Target: "../../specs/cli/spec.md#requirement-version", Spec: "specs/cli/spec.md", Anchor: "requirement-version"},
				{File: "docs/design/spec.md", Line: 6, Target: "../../specs/bench/spec.md", Spec: "specs/bench/spec.md"},
				{File: "docs/design/spec.md", Line: 7, Target: "https://github.com/mvrahden/go-test/blob/main/specs/cli/spec.md#requirement-version", Spec: "specs/cli/spec.md", Anchor: "requirement-version"},
			}, links)
		})
	})
}

func (s *LinksTestSuite) TestFences(t *gotest.T) {
	type entry struct {
		Desc  string
		Doc   string
		Lines []int
	}
	const when = "- **WHEN** x\n"
	for t, e := range gotest.Each(t, []entry{
		{Desc: "a shorter run inside a longer fence", Doc: "````\n```\n" + when + "````\n" + when, Lines: []int{5}},
		{Desc: "a run with an info string does not close", Doc: "```\n```go\n" + when + "```\n" + when, Lines: []int{5}},
		{Desc: "a longer run closes", Doc: "```\n" + when + "`````\n" + when, Lines: []int{4}},
		{Desc: "the other character does not close", Doc: "~~~\n```\n" + when + "~~~~\n" + when, Lines: []int{5}},
		{Desc: "three spaces of indent open and close", Doc: "   ```\n" + when + "   ```\n" + when, Lines: []int{4}},
		{Desc: "four spaces of indent do not open", Doc: "    ```\n" + when, Lines: []int{2}},
		{Desc: "a backtick in a backtick info string does not open", Doc: "``` a`b\n" + when, Lines: []int{2}},
		{Desc: "a backtick in a tilde info string opens", Doc: "~~~ a`b\n" + when, Lines: nil},
	}) {
		var lines []int
		for _, b := range speccatalog.WhenBullets("doc.md", []byte(e.Doc)) {
			lines = append(lines, b.Line)
		}
		gotest.Equal(t, e.Lines, lines)
	}
	t.When("a heading sits inside a longer fence after a shorter run", func(w *gotest.T) {
		anchors := speccatalog.Anchors([]byte("````\n```\n# Inside\n````\n# Outside\n"))
		w.It("has no anchor", func(it *gotest.T) {
			gotest.Equal(it, map[string]bool{"outside": true}, anchors)
		})
	})
	t.When("a clause quote sits inside a longer fence after a shorter run", func(w *gotest.T) {
		quotes := speccatalog.Quotes("doc.md", []byte("````\n```\n> when a → b\n````\n"))
		w.It("is not found", func(it *gotest.T) {
			gotest.Empty(it, quotes)
		})
	})
}

func (s *LinksTestSuite) TestDuplicateAnchors(t *gotest.T) {
	type entry struct {
		Desc     string
		Headings []string
		Anchors  []string
	}
	for t, e := range gotest.Each(t, []entry{
		{Desc: "a repeat skips a suffix a heading already took", Headings: []string{"A", "A", "A-1"}, Anchors: []string{"a", "a-1", "a-1-1"}},
		{Desc: "a repeat skips a suffix an earlier heading took", Headings: []string{"A-1", "A", "A"}, Anchors: []string{"a-1", "a", "a-2"}},
	}) {
		var doc strings.Builder
		want := map[string]bool{}
		for i, h := range e.Headings {
			doc.WriteString("# " + h + "\n")
			want[e.Anchors[i]] = true
		}
		gotest.Equal(t, want, speccatalog.Anchors([]byte(doc.String())))
	}
}

func (s *LinksTestSuite) TestSpecLinkForms(t *gotest.T) {
	type entry struct {
		Desc  string
		Line  string
		Specs []string
	}
	for t, e := range gotest.Each(t, []entry{
		{Desc: "a root-relative link", Line: "[x](/specs/cli/spec.md#nope)", Specs: []string{"specs/cli/spec.md#nope"}},
		{Desc: "a tree URL on any ref", Line: "[x](https://github.com/mvrahden/go-test/tree/main/specs/nope)", Specs: []string{"specs/nope#"}},
		{Desc: "a blob URL on a tag", Line: "[x](https://github.com/mvrahden/go-test/blob/v1.2.0/specs/cli/spec.md#a)", Specs: []string{"specs/cli/spec.md#a"}},
		{Desc: "a bare tree URL", Line: "See https://github.com/mvrahden/go-test/tree/main/specs/nope.", Specs: []string{"specs/nope#"}},
		{Desc: "a link in a code span", Line: "Write `[x](../specs/nope.md)` to link.", Specs: nil},
		{Desc: "a URL in a double-backtick span", Line: "``https://github.com/mvrahden/go-test/blob/main/specs/a.md ` ``", Specs: nil},
		{Desc: "a link after a code span", Line: "`code` and [x](../specs/a.md)", Specs: []string{"specs/a.md#"}},
		{Desc: "an unmatched backtick run is literal", Line: "`` and [x](../specs/a.md)", Specs: []string{"specs/a.md#"}},
	}) {
		var specs []string
		for _, l := range speccatalog.SpecLinks("docs/guide.md", []byte(e.Line+"\n")) {
			specs = append(specs, l.Spec+"#"+l.Anchor)
		}
		gotest.Equal(t, e.Specs, specs)
	}
}
