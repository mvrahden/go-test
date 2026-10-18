package speccatalog_test

import (
	"os"
	"path/filepath"

	"github.com/mvrahden/go-test/internal/speccatalog"
	"github.com/mvrahden/go-test/pkg/gotest"
)

const clauseSpec = "# CLI Specification\n\n## Purpose\nHow the gotest command line answers the invocations it is given.\n\n## Requirements\n\n" +
	"### Requirement: Version\nThe version subcommand SHALL print the version.\n\n" +
	"#### Scenario: Asking for the version\n" +
	"- **WHEN** the version subcommand runs\n" +
	"- **THEN** exits 0 and prints the version on stdout\n" +
	"- **AND** prints nothing on stderr\n\n" +
	"#### Scenario: Asking again\n" +
	"- **WHEN** the version subcommand runs\n" +
	"- **THEN** exits 0 and prints the version on stdout\n\n" +
	"#### Scenario: Unknown flag\n" +
	"- **WHEN** when a flag is unknown\n" +
	"- **THEN** exits 2\n\n" +
	"#### Scenario: No condition\n" +
	"- **THEN** is no clause\n"

func specClauses() []string {
	return speccatalog.Clauses([]speccatalog.Spec{speccatalog.ParseSpec("cli", "specs/cli/spec.md", []byte(clauseSpec))})
}

const sampleDoc = "# Title\n\nProse.\n\n> when the version subcommand runs → exits 0 and prints the version on stdout\n\n> when a flag is unknown → exits 2\n\n> when nothing is asked → nothing happens\n\n> not a quote\n"

// QuotesTestSuite: the specs' clauses are read as "when → it" sentences,
// and clause-shaped lines and WHEN bullets outside fences are found with
// their file and line.
type QuotesTestSuite struct{}

func (s *QuotesTestSuite) SuiteConfig() gotest.SuiteConfig {
	cfg := gotest.DefaultSuiteConfig()
	cfg.Parallel = true
	return cfg
}

func (s *QuotesTestSuite) TestClausesAreWhenArrowThen(t *gotest.T) {
	t.When("a spec states scenarios", func(w *gotest.T) {
		w.It("yields one clause per THEN and AND after its WHEN, once each, in spec order", func(it *gotest.T) {
			gotest.Equal(it, []string{
				"when the version subcommand runs → exits 0 and prints the version on stdout",
				"when the version subcommand runs → prints nothing on stderr",
				"when a flag is unknown → exits 2",
			}, specClauses())
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
	unresolved := speccatalog.Unresolved(speccatalog.Quotes("doc.md", []byte(sampleDoc)), specClauses())
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

func (s *QuotesTestSuite) TestClauseLinesResolveAgainstTheSpecs(t *gotest.T) {
	dir := t.TempDir()
	gotest.NoError(t, os.WriteFile(filepath.Join(dir, "a.patch"), []byte("Expect: x\n"+
		"Clause: when a flag is unknown → exits 2\n"+
		"Clause: when the version subcommand runs → exits 0\n"+
		"--- a/x.go\n+++ b/x.go\n"), 0o600))
	unmutated, unknown, err := speccatalog.Unmutated(specClauses(), dir)
	gotest.NoError(t, err)
	t.When("a Clause line names text no scenario states", func(w *gotest.T) {
		w.It("is reported unknown, and the named scenario clause counts as mutated", func(it *gotest.T) {
			gotest.Equal(it, []string{"a.patch: when the version subcommand runs → exits 0"}, unknown)
			gotest.NotContains(it, unmutated, "when a flag is unknown → exits 2")
			gotest.Len(it, unmutated, 2)
		})
	})
}
