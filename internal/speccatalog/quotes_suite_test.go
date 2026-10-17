package speccatalog_test

import (
	"github.com/mvrahden/go-test/internal/speccatalog"
	"github.com/mvrahden/go-test/pkg/gotest"
)

const sampleCatalog = `{"packages":[{"nodes":[{"kind":"suite","display":"CLIVersion","children":[
 {"kind":"method","display":"Version","children":[
  {"kind":"block","vocab":"when","display":"when the version subcommand runs","children":[
   {"kind":"block","vocab":"it","display":"exits 0 and prints the version on stdout","children":[]},
   {"kind":"block","vocab":"when","display":"given a broken config","children":[
    {"kind":"block","vocab":"it","display":"still prints the version","children":[]}]}]}]}]}]}]}`

const sampleDoc = "# Title\n\nProse.\n\n> when the version subcommand runs → exits 0 and prints the version on stdout\n\n> given a broken config → still prints the version\n\n> when nothing is asked → nothing happens\n\n> not a quote\n"

// QuotesTestSuite: prose may state a behavior only by quoting a clause the
// catalog holds, and an unresolved quote is named with its file and line.
type QuotesTestSuite struct{}

func (s *QuotesTestSuite) SuiteConfig() gotest.SuiteConfig {
	cfg := gotest.DefaultSuiteConfig()
	cfg.Parallel = true
	return cfg
}

func (s *QuotesTestSuite) TestClausesAreWhenArrowIt(t *gotest.T) {
	clauses, err := speccatalog.Clauses([]byte(sampleCatalog))
	gotest.NoError(t, err)
	t.When("a catalog holds nested When blocks", func(w *gotest.T) {
		w.It("yields one clause per It under its innermost When", func(it *gotest.T) {
			gotest.ElementsMatch(it, []string{
				"when the version subcommand runs → exits 0 and prints the version on stdout",
				"given a broken config → still prints the version",
			}, clauses)
		})
	})
}

func (s *QuotesTestSuite) TestQuotesAreExtractedWithTheirLine(t *gotest.T) {
	quotes := speccatalog.Quotes("doc.md", []byte(sampleDoc))
	t.When("a document holds blockquotes", func(w *gotest.T) {
		w.It("keeps the clause-shaped ones with their line numbers", func(it *gotest.T) {
			gotest.Len(it, quotes, 3)
			gotest.Equal(it, speccatalog.Quote{File: "doc.md", Line: 5, Text: "when the version subcommand runs → exits 0 and prints the version on stdout"}, quotes[0])
			gotest.Equal(it, 9, quotes[2].Line)
		})
	})
}

func (s *QuotesTestSuite) TestUnresolvedQuotesAreNamed(t *gotest.T) {
	clauses, err := speccatalog.Clauses([]byte(sampleCatalog))
	gotest.NoError(t, err)
	unresolved := speccatalog.Unresolved(speccatalog.Quotes("doc.md", []byte(sampleDoc)), clauses)
	t.When("a quote has no clause", func(w *gotest.T) {
		w.It("is reported by file, line and text, and resolved quotes are not", func(it *gotest.T) {
			gotest.Equal(it, []string{"doc.md:9: when nothing is asked → nothing happens"}, unresolved)
		})
	})
}
