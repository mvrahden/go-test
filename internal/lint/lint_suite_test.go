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

func (s *LintTestSuite) TestAnalyzer(t *gotest.T) {
	testdata := analysistest.TestData()

	t.When("sample code", func(w *gotest.T) {
		w.It("detects violations", func(it *gotest.T) {
			analysistest.Run(it.T(), testdata, lint.Analyzer, "sample")
		})
	})

	t.When("testify imports", func(w *gotest.T) {
		w.It("detects testify usage", func(it *gotest.T) {
			analysistest.Run(it.T(), testdata, lint.Analyzer, "withtestify")
		})
	})

	t.When("nolint comments", func(w *gotest.T) {
		w.It("respects inline nolint", func(it *gotest.T) {
			analysistest.Run(it.T(), testdata, lint.Analyzer, "withnolint")
		})
	})

	t.When("foreign lookalikes", func(w *gotest.T) {
		w.It("ignores assertion and polling names from other packages", func(it *gotest.T) {
			analysistest.Run(it.T(), testdata, lint.Analyzer, "withforeign")
		})
	})

	t.When("poll scope", func(w *gotest.T) {
		w.It("detects poll scope violations", func(it *gotest.T) {
			analysistest.Run(it.T(), testdata, lint.Analyzer, "withpollscope")
		})
	})

	t.When("assertion simplify", func(w *gotest.T) {
		w.It("detects sub-optimal assertion patterns", func(it *gotest.T) {
			analysistest.RunWithSuggestedFixes(it.T(), testdata, lint.Analyzer, "withsimplify")
		})
	})

	t.When("fail guard", func(w *gotest.T) {
		w.It("detects if+Fail guards that assertions express directly", func(it *gotest.T) {
			analysistest.RunWithSuggestedFixes(it.T(), testdata, lint.Analyzer, "withfailguard", "withfailguard_noimport")
		})
	})

	t.When("assertion redundant", func(w *gotest.T) {
		w.It("detects redundant guard assertions before stronger ones", func(it *gotest.T) {
			analysistest.RunWithSuggestedFixes(it.T(), testdata, lint.Analyzer, "withredundant")
		})
	})

	t.When("suite cleanup", func(w *gotest.T) {
		w.It("detects .T().Cleanup in suite methods", func(it *gotest.T) {
			analysistest.Run(it.T(), testdata, lint.Analyzer, "withcleanup")
		})
	})

	t.When("suite direct calls", func(w *gotest.T) {
		w.It("detects T.Parallel and T.Run in suite methods", func(it *gotest.T) {
			analysistest.Run(it.T(), testdata, lint.Analyzer, "withdirectcalls")
		})
	})

	t.When("t escape", func(w *gotest.T) {
		w.It("detects unnecessary .T() escape and applies fixes", func(it *gotest.T) {
			analysistest.RunWithSuggestedFixes(it.T(), testdata, lint.Analyzer, "withtescape")
		})
	})

	t.When("file-level nolint", func(w *gotest.T) {
		w.It("respects file-level nolint", func(it *gotest.T) {
			analysistest.Run(it.T(), testdata, lint.Analyzer, "withnolint_file")
		})
	})

	t.When("benchmark methods", func(w *gotest.T) {
		w.It("detects bench-loop and bench-fixture-io violations", func(it *gotest.T) {
			analysistest.Run(it.T(), testdata, lint.Analyzer, "bench")
		})
	})

	t.When("fuzz methods", func(w *gotest.T) {
		w.It("detects fuzz-determinism, fuzz-no-oracle, and fuzz-seed violations", func(it *gotest.T) {
			analysistest.Run(it.T(), testdata, lint.Analyzer, "fuzz")
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
			for _, rule := range []lint.Rule{lint.Focus, lint.PollScope, lint.TestSignature, lint.SuiteLifecycle} {
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
