package lint //nolint:stdlib-test

import (
	"testing"

	"golang.org/x/tools/go/analysis/analysistest"
)

func TestAnalyzer(t *testing.T) {
	testdata := analysistest.TestData()
	analysistest.Run(t, testdata, Analyzer, "sample")
}

func TestAnalyzer_Testify(t *testing.T) {
	testdata := analysistest.TestData()
	analysistest.Run(t, testdata, Analyzer, "withtestify")
}

func TestAnalyzer_Nolint(t *testing.T) {
	testdata := analysistest.TestData()
	analysistest.Run(t, testdata, Analyzer, "withnolint")
}

func TestAnalyzer_PollScope(t *testing.T) {
	testdata := analysistest.TestData()
	analysistest.Run(t, testdata, Analyzer, "withpollscope")
}

func TestAnalyzer_NolintFile(t *testing.T) {
	testdata := analysistest.TestData()
	analysistest.Run(t, testdata, Analyzer, "withnolint_file")
}

func TestParseNolint(t *testing.T) {
	tests := []struct {
		text      string
		wantOk    bool
		wantRules map[Rule]bool
	}{
		{"//nolint", true, nil},
		{"//nolint:stdlib-test", true, map[Rule]bool{StdlibTest: true}},
		{"//nolint:stdlib-test,testify", true, map[Rule]bool{StdlibTest: true, Testify: true}},
		{"//nolint:stdlib-test // legacy test", true, map[Rule]bool{StdlibTest: true}},
		{"//nolint:", true, nil},
		{"// some comment", false, nil},
		{"//nolinting is fun", false, nil},
		{"//nolint:focus", true, map[Rule]bool{Focus: true}},
	}
	for _, tt := range tests {
		rules, ok := parseNolint(tt.text)
		if ok != tt.wantOk {
			t.Errorf("parseNolint(%q) ok = %v, want %v", tt.text, ok, tt.wantOk)
			continue
		}
		if !ok {
			continue
		}
		if tt.wantRules == nil && rules != nil {
			t.Errorf("parseNolint(%q) rules = %v, want nil (blanket)", tt.text, rules)
		}
		if tt.wantRules != nil {
			if len(rules) != len(tt.wantRules) {
				t.Errorf("parseNolint(%q) rules = %v, want %v", tt.text, rules, tt.wantRules)
				continue
			}
			for r := range tt.wantRules {
				if !rules[r] {
					t.Errorf("parseNolint(%q) missing rule %q", tt.text, r)
				}
			}
		}
	}
}
