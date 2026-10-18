package speccatalog_test

import (
	"github.com/mvrahden/go-test/internal/speccatalog"
	"github.com/mvrahden/go-test/pkg/gotest"
)

const benchSpec = "# Bench Specification\n\n## Purpose\nHow bench compares runs.\n\n## Requirements\n\n" +
	"### Requirement: Gate\nA zero gate SHALL compare without a gate.\n\n" +
	"#### Scenario: A zero gate\n- **WHEN** --gate=0 is given\n- **THEN** compares without a gate\n"

// MutantsTestSuite: a mutant's Clause lines name the conformance packages
// that judge it, unless the mutant lives in code a test binary links.
type MutantsTestSuite struct{}

func (s *MutantsTestSuite) SuiteConfig() gotest.SuiteConfig {
	cfg := gotest.DefaultSuiteConfig()
	cfg.Parallel = true
	return cfg
}

func (s *MutantsTestSuite) TestPackagesOfClauses(t *gotest.T) {
	specs := []speccatalog.Spec{
		speccatalog.ParseSpec("cli", "specs/cli/spec.md", []byte(sampleSpec)),
		speccatalog.ParseSpec("bench", "specs/bench/spec.md", []byte(benchSpec)),
	}
	t.When("a patch names clauses of two specs", func(w *gotest.T) {
		patch := "Expect: x\nClause: when the version subcommand runs → exits 0 and prints the version on stdout\nClause: when --gate=0 is given → compares without a gate\nClause: when the version subcommand runs → prints nothing on stderr\n--- a/cmd/gotest/bench.go\n+++ b/cmd/gotest/bench.go\n"
		pkgs, unknown := speccatalog.PackagesOf(specs, []byte(patch))
		w.It("lists both package directories once each, sorted", func(it *gotest.T) {
			gotest.Equal(it, []string{"./tests/conformance/bench", "./tests/conformance/cli"}, pkgs)
			gotest.Empty(it, unknown)
		})
	})
	t.When("a patch names a clause no scenario states", func(w *gotest.T) {
		patch := "Expect: x\nClause: when nothing is stated → is no clause\n--- a/cmd/gotest/bench.go\n+++ b/cmd/gotest/bench.go\n"
		pkgs, unknown := speccatalog.PackagesOf(specs, []byte(patch))
		w.It("reports it unknown and adds no package", func(it *gotest.T) {
			gotest.Equal(it, []string{"when nothing is stated → is no clause"}, unknown)
			gotest.Empty(it, pkgs)
		})
	})
}

func (s *MutantsTestSuite) TestLinksIntoTestBinaries(t *gotest.T) {
	t.When("a patch touches pkg/gotest and cmd/gotest", func(w *gotest.T) {
		patch := "Expect: x\n--- a/cmd/gotest/bench.go\n+++ b/cmd/gotest/bench.go\n@@ -1 +1 @@\n--- a/pkg/gotest/assert.go\n+++ b/pkg/gotest/assert.go\n"
		w.It("links into test binaries", func(it *gotest.T) {
			gotest.True(it, speccatalog.LinksIntoTestBinaries([]byte(patch)))
		})
	})
	t.When("a patch touches only cmd/gotest", func(w *gotest.T) {
		patch := "Expect: x\n--- a/cmd/gotest/bench.go\n+++ b/cmd/gotest/bench.go\n@@ -1 +1 @@\n-x\n+y\n"
		w.It("does not", func(it *gotest.T) {
			gotest.False(it, speccatalog.LinksIntoTestBinaries([]byte(patch)))
		})
	})
}
