package gotestrunner

import (
	"fmt"

	"github.com/mvrahden/go-test/internal/cmd/testgen"
	"github.com/mvrahden/go-test/internal/gotestgen"
)

func SuitesGenerate(pkgPattern string) (gotestgen.GenerateResults, error) {
	results, err := testgen.GenerateSuites(pkgPattern)
	if err != nil {
		return nil, fmt.Errorf("failed generating suites: %w", err)
	}
	return results, nil
}

// SuitesGenerateWithCollectorResults generates test suites and also returns
// the raw collector results, which can be used to discover shared fixtures.
func SuitesGenerateWithCollectorResults(pkgPattern string) (gotestgen.GenerateResults, []gotestgen.CollectorResult, error) {
	results, collectorResults, err := testgen.GenerateSuitesWithCollectorResults(pkgPattern)
	if err != nil {
		return nil, nil, fmt.Errorf("failed generating suites: %w", err)
	}
	return results, collectorResults, nil
}
