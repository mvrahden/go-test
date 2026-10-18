package speccatalog_test

import (
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
