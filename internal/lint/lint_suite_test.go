package lint_test

import (
	"github.com/mvrahden/go-test/internal/lint"
	"github.com/mvrahden/go-test/pkg/gotest"
	"golang.org/x/tools/go/analysis/analysistest"
)

// LintTestSuite tests the gotest lint analyzer rules and nolint directive parsing.
type LintTestSuite struct{}

func (s *LintTestSuite) SuiteConfig() gotest.SuiteConfig {
	return gotest.SuiteConfig{Parallel: true}
}

// analysistest type-checks the fixture package and everything it imports, which
// dwarfs the analysis itself: a package loaded on its own measured ~2.2s against
// ~0.2s marginal once batched into a shared load. So the rules are grouped by
// call, not split by rule.
//
// Two calls because a call has one mode. Checking fixes over the whole set would
// demand a .golden for every broad fixture, tying sample's expected rewrite to
// the fix output of every rule that fires in it.

// diagnosticFixtures are the packages whose want comments are the whole
// expectation; none of them records a suggested fix.
var diagnosticFixtures = []string{
	"sample",
	"withtestify",
	"withnolint",
	"withforeign",
	"withpollscope",
	"withcleanup",
	"withdirectcalls",
	"withnolint_file",
	"withsharedfixture",
}

// rewriteFixtures are the packages that additionally pin the rewrite a rule
// offers, each against its own .golden.
var rewriteFixtures = []string{
	"withsimplify",
	"withfailguard",
	"withfailguard_noimport",
	"withredundant",
	"withtescape",
}

func (s *LintTestSuite) TestDiagnostics(t *gotest.T) {
	t.When("every rule's fixture package", func(w *gotest.T) {
		w.It("reports exactly the diagnostics its want comments declare", func(it *gotest.T) {
			analysistest.Run(it.T(), analysistest.TestData(), lint.Analyzer, diagnosticFixtures...)
		})
	})
}

func (s *LintTestSuite) TestSuggestedFixes(t *gotest.T) {
	t.When("a rule that offers a rewrite", func(w *gotest.T) {
		w.It("produces the fix recorded in the golden file", func(it *gotest.T) {
			analysistest.RunWithSuggestedFixes(it.T(), analysistest.TestData(), lint.Analyzer, rewriteFixtures...)
		})
	})
}

func (s *LintTestSuite) TestDisableNolintFlag(t *gotest.T) {
	t.When("analyzer flags", func(w *gotest.T) {
		w.It("registers the disable-nolint flag", func(it *gotest.T) {
			f := lint.Analyzer.Flags.Lookup("disable-nolint")
			gotest.NotZero(it, f)
			gotest.Equal(it, "false", f.DefValue)
		})
	})
}

func (s *LintTestSuite) TestTierPolicy(t *gotest.T) {
	t.When("tier-derived skip surface", func(w *gotest.T) {
		w.It("registers a skip flag for every non-integrity rule and none for integrity rules", func(it *gotest.T) {
			for _, rule := range []lint.Rule{lint.StdlibTest, lint.Testify, lint.AssertionSimplify, lint.AssertionRedundant, lint.FailGuard, lint.TEscape} {
				gotest.NotZero(it, lint.Analyzer.Flags.Lookup("skip-"+string(rule)), "missing skip flag for %s", rule)
				gotest.True(it, lint.SkippableRules[rule], "rule %s should be skippable", rule)
			}
			for _, rule := range []lint.Rule{lint.Focus, lint.PollScope, lint.TestSignature, lint.SuiteLifecycle, lint.SharedFixtureUndeclared} {
				gotest.Zero(it, lint.Analyzer.Flags.Lookup("skip-"+string(rule)), "unexpected skip flag for %s", rule)
				gotest.False(it, lint.SkippableRules[rule], "integrity rule %s must not be skippable", rule)
			}
			gotest.True(it, lint.Known(lint.Focus))
			gotest.False(it, lint.Known(lint.Rule("nope")))
		})
	})
}

func (s *LintTestSuite) TestParseNolint(t *gotest.T) {
	tests := []struct {
		desc      string
		text      string
		wantOk    bool
		wantRules map[lint.Rule]bool
	}{
		{"blanket nolint", "//nolint", true, nil},
		{"blanket nolint with space", "// nolint", true, nil},
		{"spaced nolint with rule", "// nolint:stdlib-test", true, map[lint.Rule]bool{lint.StdlibTest: true}},
		{"single rule", "//nolint:stdlib-test", true, map[lint.Rule]bool{lint.StdlibTest: true}},
		{"multiple rules", "//nolint:stdlib-test,testify", true, map[lint.Rule]bool{lint.StdlibTest: true, lint.Testify: true}},
		{"with trailing comment", "//nolint:stdlib-test // legacy test", true, map[lint.Rule]bool{lint.StdlibTest: true}},
		{"empty rules", "//nolint:", true, nil},
		{"regular comment", "// some comment", false, nil},
		{"nolint-like", "//nolinting is fun", false, nil},
		{"focus rule", "//nolint:focus", true, map[lint.Rule]bool{lint.Focus: true}},
	}

	for _, tc := range tests {
		t.When(tc.desc, func(w *gotest.T) {
			w.It("parses correctly", func(it *gotest.T) {
				rules, ok := lint.ExportParseNolint(tc.text)
				gotest.Equal(it, tc.wantOk, ok)
				if !ok {
					return
				}
				if tc.wantRules == nil {
					gotest.Empty(it, rules)
				} else {
					gotest.Len(it, tc.wantRules, len(rules))
					for r := range tc.wantRules {
						gotest.True(it, rules[r])
					}
				}
			})
		})
	}
}

// SkippedStyleTestSuite runs separately and non-parallel because it
// temporarily mutates the shared analyzer flags.
type SkippedStyleTestSuite struct{}

func (s *SkippedStyleTestSuite) BeforeEach(_ *gotest.T) { lint.ExportSetSkip(lint.FailGuard, true) }
func (s *SkippedStyleTestSuite) AfterEach(_ *gotest.T)  { lint.ExportSetSkip(lint.FailGuard, false) }

func (s *SkippedStyleTestSuite) TestSkipSilencesExpressivenessRule(t *gotest.T) {
	testdata := analysistest.TestData()

	t.When("skip-fail-guard is set", func(w *gotest.T) {
		w.It("reports nothing for fail-guard findings", func(it *gotest.T) {
			analysistest.Run(it.T(), testdata, lint.Analyzer, "withskippedstyle")
		})
	})
}

// SkippedEscapeTestSuite runs separately and non-parallel because it
// temporarily mutates the shared analyzer flags.
type SkippedEscapeTestSuite struct{}

func (s *SkippedEscapeTestSuite) BeforeEach(_ *gotest.T) { lint.ExportSetSkip(lint.TEscape, true) }
func (s *SkippedEscapeTestSuite) AfterEach(_ *gotest.T)  { lint.ExportSetSkip(lint.TEscape, false) }

func (s *SkippedEscapeTestSuite) TestSkippedRuleReleasesClaims(t *gotest.T) {
	testdata := analysistest.TestData()

	t.When("skip-t-escape is set", func(w *gotest.T) {
		w.It("lets fail-guard take over escaped halting guards", func(it *gotest.T) {
			analysistest.Run(it.T(), testdata, lint.Analyzer, "withskippedtescape")
		})
	})
}

// DisableNolintTestSuite runs separately and non-parallel because it
// temporarily mutates the shared analyzer flags.
type DisableNolintTestSuite struct{}

func (s *DisableNolintTestSuite) BeforeEach(_ *gotest.T) { lint.ExportSetDisableNolint(true) }
func (s *DisableNolintTestSuite) AfterEach(_ *gotest.T)  { lint.ExportSetDisableNolint(false) }

func (s *DisableNolintTestSuite) TestNolintDirectivesIgnored(t *gotest.T) {
	testdata := analysistest.TestData()

	t.When("disable-nolint is set", func(w *gotest.T) {
		w.It("reports all diagnostics regardless of nolint directives", func(it *gotest.T) {
			analysistest.Run(it.T(), testdata, lint.Analyzer, "withdisablednolint")
		})
	})
}
