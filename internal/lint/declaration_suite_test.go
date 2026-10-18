package lint_test

import (
	"github.com/mvrahden/go-test/internal/lint"
	"github.com/mvrahden/go-test/pkg/gotest"
	"golang.org/x/tools/go/analysis/analysistest"
)

// DeclarationRulesTestSuite holds the rules for how suites and fixtures are
// declared, composed and driven: suite-declaration, no-embedding and
// fixture-hook-call.
type DeclarationRulesTestSuite struct{}

func (s *DeclarationRulesTestSuite) SuiteConfig() gotest.SuiteConfig {
	cfg := gotest.IntegrationSuiteConfig()
	cfg.Parallel = true
	return cfg
}

func (s *DeclarationRulesTestSuite) TestSuiteDeclaration(t *gotest.T) {
	t.When("a suite or a bound fixture is declared inside a type block", func(w *gotest.T) {
		w.It("is reported at its declaration and moved into its own, doc comment kept", func(it *gotest.T) {
			analysistest.RunWithSuggestedFixes(it.T(), analysistest.TestData(), lint.Analyzer, "withdeclaration")
		})
		w.It("leaves source that type-checks", func(it *gotest.T) {
			gotest.Empty(it, goldenTypeErrors(analysistest.TestData(), "withdeclaration"))
		})
	})
}

func (s *DeclarationRulesTestSuite) TestNoEmbedding(t *gotest.T) {
	t.When("a suite or a bound fixture embeds a type or holds a suite", func(w *gotest.T) {
		w.It("reports each such field with the alternative, and no plain helper", func(it *gotest.T) {
			analysistest.Run(it.T(), analysistest.TestData(), lint.Analyzer, "withembedding")
		})
	})
}

func (s *DeclarationRulesTestSuite) TestFixtureHookCall(t *gotest.T) {
	t.When("a suite or fixture method calls a lifecycle hook", func(w *gotest.T) {
		w.It("reports a hook of a bound fixture or of its own, and no other call", func(it *gotest.T) {
			analysistest.Run(it.T(), analysistest.TestData(), lint.Analyzer, "withhookcall")
		})
	})
}

func (s *DeclarationRulesTestSuite) TestTiers(t *gotest.T) {
	t.When("the declaration rules are registered", func(w *gotest.T) {
		w.It("are integrity rules with no skip flag", func(it *gotest.T) {
			for _, rule := range []lint.Rule{lint.SuiteDeclaration, lint.NoEmbedding, lint.FixtureHookCall} {
				gotest.True(it, lint.Known(rule), "rule %s is not registered", rule)
				gotest.False(it, lint.SkippableRules[rule], "integrity rule %s must not be skippable", rule)
				gotest.Zero(it, lint.Analyzer.Flags.Lookup("skip-"+string(rule)))
			}
		})
	})
}
