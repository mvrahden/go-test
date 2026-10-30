package lint_test

import (
	"bytes"
	"encoding/json"
	"go/ast"
	"go/parser"
	"go/token"
	"slices"
	"strings"
	"time"

	"github.com/mvrahden/go-test/internal/lint"
	"github.com/mvrahden/go-test/pkg/gotest"
	"golang.org/x/tools/go/analysis/analysistest"
)

// RulesTestSuite pins the rule registry to what the rules do: a rule is
// fixable exactly when the fixtures see it attach a suggested fix.
type RulesTestSuite struct{}

func (s *RulesTestSuite) SuiteConfig() gotest.SuiteConfig {
	return gotest.SuiteConfig{Parallel: true, Timeout: 3 * time.Minute}
}

func (s *RulesTestSuite) TestFixableMeansARuleAttachesAFix(t *gotest.T) {
	var doc struct {
		Rules []struct {
			ID      string `json:"id"`
			Fixable bool   `json:"fixable"`
		} `json:"rules"`
	}
	var buf bytes.Buffer
	gotest.NoError(t, lint.WriteRulesJSON(&buf))
	gotest.NoError(t, json.Unmarshal(buf.Bytes(), &doc))
	var declared []string
	for _, r := range doc.Rules {
		if r.Fixable {
			declared = append(declared, r.ID)
		}
	}

	fixtures := append(slices.Clone(diagnosticFixtures), rewriteFixtures...)
	// The rules with a suite of their own, whose fixtures the lists above
	// leave out.
	fixtures = append(fixtures, "withdeclaration", "withembedding", "withhookcall", "withgeneration")
	results := analysistest.Run(t.T(), analysistest.TestData(), lint.Analyzer, fixtures...)
	seen := map[string]bool{}
	for _, res := range results {
		for _, d := range res.Diagnostics {
			if len(d.SuggestedFixes) > 0 {
				seen[d.Category] = true
			}
		}
	}
	var fixing []string
	for id := range seen {
		fixing = append(fixing, id)
	}
	slices.Sort(declared)
	slices.Sort(fixing)

	t.When("every rule's fixture package is analyzed", func(w *gotest.T) {
		w.It("sees a fix attached by exactly the rules the registry calls fixable", func(it *gotest.T) {
			gotest.Equal(it, declared, fixing)
		})
	})
}

// declaredRules reads the Rule constants from the source, apart from the
// registry, so a constant the registry lacks shows.
func declaredRules(t *gotest.T) []string {
	file, err := parser.ParseFile(token.NewFileSet(), "lint.go", nil, 0)
	gotest.NoError(t, err)
	var ids []string
	for _, decl := range file.Decls {
		gd, ok := decl.(*ast.GenDecl)
		if !ok || gd.Tok != token.CONST {
			continue
		}
		for _, spec := range gd.Specs {
			vs := spec.(*ast.ValueSpec)
			if id, ok := vs.Type.(*ast.Ident); !ok || id.Name != "Rule" {
				continue
			}
			for _, v := range vs.Values {
				lit, ok := v.(*ast.BasicLit)
				gotest.True(t, ok, "a Rule constant that is no literal")
				ids = append(ids, strings.Trim(lit.Value, `"`))
			}
		}
	}
	slices.Sort(ids)
	return ids
}

func (s *RulesTestSuite) TestTheRegistryListsEachRuleOnce(t *gotest.T) {
	var buf bytes.Buffer
	lint.WriteRuleTable(&buf)
	var listed []string
	for _, line := range strings.Split(strings.TrimSuffix(buf.String(), "\n"), "\n") {
		if !strings.HasPrefix(line, "    ") {
			listed = append(listed, strings.Fields(line)[0])
		}
	}
	slices.Sort(listed)
	var ids []string
	for _, r := range lint.RuleIDs() {
		ids = append(ids, string(r))
	}
	declared := declaredRules(t)
	t.When("the rule table and the registry are read against the Rule constants", func(w *gotest.T) {
		w.It("names every constant once", func(it *gotest.T) {
			gotest.NotEmpty(it, declared)
			gotest.Equal(it, declared, listed)
			gotest.Equal(it, declared, ids)
		})
	})
}
