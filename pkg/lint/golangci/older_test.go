package golangci_test

import (
	"flag"
	"testing"

	"golang.org/x/tools/go/analysis"

	"github.com/mvrahden/go-test/pkg/gotest"
	"github.com/mvrahden/go-test/pkg/lint/golangci"
)

// olderAnalyzer stands in for the analyzer of a gotest without -nolint-allow.
func olderAnalyzer() *analysis.Analyzer {
	fs := flag.NewFlagSet("gotestlint", flag.ContinueOnError)
	fs.Bool("skip-testify", false, "")
	return &analysis.Analyzer{Name: "gotestlint", Doc: "older", Flags: *fs, Run: func(*analysis.Pass) (any, error) { return nil, nil }}
}

func TestOlderAnalyzer(t *testing.T) {
	_, err := golangci.NewFor(olderAnalyzer(), map[string]any{})
	gotest.NoError(t, err)
	_, err = golangci.NewFor(olderAnalyzer(), nil)
	gotest.NoError(t, err)
	_, err = golangci.NewFor(olderAnalyzer(), map[string]any{"skip": []any{"testify"}})
	gotest.NoError(t, err)
	_, err = golangci.NewFor(olderAnalyzer(), map[string]any{"nolint-allow": []any{"nilaway"}})
	gotest.ErrorContains(t, err, "setting nolint-allow needs gotest v1.38.0 or later")
}
