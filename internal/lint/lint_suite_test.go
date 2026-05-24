package lint_test

import (
	"github.com/mvrahden/go-test/internal/lint"
	"github.com/mvrahden/go-test/pkg/gotest"
	"golang.org/x/tools/go/analysis/analysistest"
)

type LintTestSuite struct{}

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

	t.When("file-level nolint", func(w *gotest.T) {
		w.It("respects file-level nolint", func(it *gotest.T) {
			analysistest.Run(it.T(), testdata, lint.Analyzer, "withnolint_file")
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
					gotest.True(it, rules == nil)
				} else {
					gotest.Equal(it, len(tc.wantRules), len(rules))
					for r := range tc.wantRules {
						gotest.True(it, rules[r])
					}
				}
			})
		})
	}
}
