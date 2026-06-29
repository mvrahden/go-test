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

	t.When("suite cleanup", func(w *gotest.T) {
		w.It("detects .T().Cleanup in suite methods", func(it *gotest.T) {
			analysistest.Run(it.T(), testdata, lint.Analyzer, "withcleanup")
		})
	})

	t.When("file-level nolint", func(w *gotest.T) {
		w.It("respects file-level nolint", func(it *gotest.T) {
			analysistest.Run(it.T(), testdata, lint.Analyzer, "withnolint_file")
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

// DisableNolintTestSuite runs separately and non-parallel because it
// temporarily mutates the shared analyzer flag.
type DisableNolintTestSuite struct{}

func (s *DisableNolintTestSuite) TestNolintDirectivesIgnored(t *gotest.T) {
	testdata := analysistest.TestData()

	lint.ExportSetDisableNolint(true)
	defer lint.ExportSetDisableNolint(false)

	t.When("disable-nolint is set", func(w *gotest.T) {
		w.It("reports all diagnostics regardless of nolint directives", func(it *gotest.T) {
			analysistest.Run(it.T(), testdata, lint.Analyzer, "withdisablednolint")
		})
	})
}
