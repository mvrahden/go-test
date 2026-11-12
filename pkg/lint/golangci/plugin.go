// Package golangci is gotest's analyzer as a golangci-lint module plugin.
//
// Build a golangci-lint with it through .custom-gcl.yml:
//
//	version: <the golangci-lint version>
//	plugins:
//	  - module: github.com/mvrahden/go-test/pkg/lint/golangci
//	    version: <the gotest version go.mod requires>
//
// then run golangci-lint custom, and enable it in .golangci.yml:
//
//	linters:
//	  enable: [gotestlint]
//	  settings:
//	    custom:
//	      gotestlint:
//	        type: module
//	        settings:
//	          skip: [testify]          # rules to skip, as lint.skip in .gotest.yml
//	          nolint-allow: [nilaway]  # linters a nolint comment may name
//
// Each finding's message ends with its rule in parentheses: golangci-lint
// names the linter, gotestlint, not the rule. The package documentation of
// github.com/mvrahden/go-test/pkg/lint says which version to pin and how
// nolint comments read under golangci-lint.
package golangci

import (
	"fmt"
	"strings"

	"github.com/golangci/plugin-module-register/register"
	"golang.org/x/tools/go/analysis"

	"github.com/mvrahden/go-test/pkg/lint"
)

func init() { register.Plugin("gotestlint", New) }

// Settings are the plugin's settings in .golangci.yml.
type Settings struct {
	Skip        []string `json:"skip"`         // rules to skip, as lint.skip in .gotest.yml
	NolintAllow []string `json:"nolint-allow"` // linters a nolint comment may name
}

// New builds the plugin; golangci-lint calls it with the linter's settings.
func New(conf any) (register.LinterPlugin, error) {
	s, err := register.DecodeSettings[Settings](conf)
	if err != nil {
		return nil, err
	}
	for _, rule := range s.Skip {
		if lint.Analyzer.Flags.Lookup("skip-"+rule) == nil {
			return nil, fmt.Errorf("gotestlint: cannot skip %q: no such rule, or an integrity rule, which only a //nolint on its line suppresses", rule)
		}
	}
	for _, rule := range s.Skip {
		if err := lint.Analyzer.Flags.Set("skip-"+rule, "true"); err != nil {
			return nil, err
		}
	}
	if err := lint.Analyzer.Flags.Set("nolint-allow", strings.Join(s.NolintAllow, ",")); err != nil {
		return nil, err
	}
	return plugin{}, nil
}

type plugin struct{}

// BuildAnalyzers returns the analyzer, its findings named by rule.
func (plugin) BuildAnalyzers() ([]*analysis.Analyzer, error) {
	a := *lint.Analyzer
	a.Run = func(pass *analysis.Pass) (any, error) {
		p := *pass
		p.Report = func(d analysis.Diagnostic) {
			if d.Category != "" {
				d.Message += " (" + d.Category + ")"
			}
			pass.Report(d)
		}
		return lint.Analyzer.Run(&p)
	}
	return []*analysis.Analyzer{&a}, nil
}

// GetLoadMode asks for type information: most rules need it.
func (plugin) GetLoadMode() string { return register.LoadModeTypesInfo }
