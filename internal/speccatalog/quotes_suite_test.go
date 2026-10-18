package speccatalog_test

import (
	"os"
	"path/filepath"

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

// QuotesTestSuite: the catalog's clauses are read as "when → it" sentences,
// and clause-shaped lines and WHEN bullets outside fences are found with
// their file and line.
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

const excludedCatalog = `{"packages":[{"nodes":[{"kind":"suite","display":"Parked","excluded":true,"children":[
 {"kind":"method","display":"Parked","children":[
  {"kind":"block","vocab":"when","display":"when the suite is parked","children":[
   {"kind":"block","vocab":"it","display":"is not a sentence anyone may quote","children":[]}]}]}]},
 {"kind":"suite","display":"Live","children":[
 {"kind":"method","display":"Live","children":[
  {"kind":"block","vocab":"when","display":"when the suite runs","children":[
   {"kind":"block","vocab":"it","display":"is a sentence","children":[]}]}]}]}]}]}`

func (s *QuotesTestSuite) TestExcludedSuitesYieldNoClauses(t *gotest.T) {
	clauses, err := speccatalog.Clauses([]byte(excludedCatalog))
	gotest.NoError(t, err)
	t.When("a catalog holds an excluded suite beside a live one", func(w *gotest.T) {
		w.It("yields the live suite's clauses alone", func(it *gotest.T) {
			gotest.Equal(it, []string{"when the suite runs → is a sentence"}, clauses)
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

const fencedDoc = "> when outside → is found\n" +
	"```markdown\n" +
	"> when fenced → is skipped\n" +
	"```\n" +
	"> when after the fence → is found\n" +
	"  ```\n" +
	"> when unterminated → is skipped\n"

func (s *QuotesTestSuite) TestQuotesInFencesAreSkipped(t *gotest.T) {
	quotes := speccatalog.Quotes("doc.md", []byte(fencedDoc))
	t.When("a document holds clause-shaped lines inside code fences", func(w *gotest.T) {
		w.It("finds only the lines outside them, and an unterminated fence runs to the end", func(it *gotest.T) {
			gotest.Equal(it, []speccatalog.Quote{
				{File: "doc.md", Line: 1, Text: "when outside → is found"},
				{File: "doc.md", Line: 5, Text: "when after the fence → is found"},
			}, quotes)
		})
	})
}

const bulletDoc = "- **WHEN** a guide restates a scenario\n" +
	"- **THEN** is found\n" +
	"```\n- **WHEN** fenced\n```\n" +
	"Prose naming **WHEN** bullets.\n" +
	"* **WHEN** starred\n"

func (s *QuotesTestSuite) TestWhenBulletsAreFound(t *gotest.T) {
	bullets := speccatalog.WhenBullets("doc.md", []byte(bulletDoc))
	t.When("a document holds WHEN bullets inside and outside fences", func(w *gotest.T) {
		w.It("finds the bullets outside them with their lines", func(it *gotest.T) {
			gotest.Equal(it, []speccatalog.Quote{
				{File: "doc.md", Line: 1, Text: "- **WHEN** a guide restates a scenario"},
				{File: "doc.md", Line: 7, Text: "* **WHEN** starred"},
			}, bullets)
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

func (s *QuotesTestSuite) TestMutantsNameTheClausesTheyFalsify(t *gotest.T) {
	dir := t.TempDir()
	gotest.NoError(t, os.WriteFile(filepath.Join(dir, "a.patch"), []byte("Expect: x\nClause: when a → b\nClause: when c → d\n--- a/x.go\n+++ b/x.go\n"), 0o600))
	gotest.NoError(t, os.WriteFile(filepath.Join(dir, "b.patch"), []byte("Expect: y\nClause: when a → b\n--- a/y.go\n+++ b/y.go\n"), 0o600))
	gotest.NoError(t, os.WriteFile(filepath.Join(dir, "notes.txt"), []byte("Clause: when ignored → file\n"), 0o600))
	clauses := []string{"when a → b", "when e → f", "when g → h"}
	unmutated, unknown, err := speccatalog.Unmutated(clauses, dir)
	gotest.NoError(t, err)
	t.When("the mutants' Clause lines are read against the catalog", func(w *gotest.T) {
		w.It("lists the clauses no patch names, sorted", func(it *gotest.T) {
			gotest.Equal(it, []string{"when e → f", "when g → h"}, unmutated)
		})
		w.It("names a Clause line the catalog does not hold, with its patch", func(it *gotest.T) {
			gotest.Equal(it, []string{"a.patch: when c → d"}, unknown)
		})
	})
}
