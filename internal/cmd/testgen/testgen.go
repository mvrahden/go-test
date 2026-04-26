package testgen

import (
	"fmt"

	"github.com/mvrahden/go-test/internal/gotestgen"
)

func GenerateSuites(path string) (gotestgen.GenerateResults, error) {
	res, err := gotestgen.Generate(path)
	if err != nil {
		return nil, fmt.Errorf("failed generating suites: %w", err)
	}

	return res, nil
}

// GenerateSuitesWithCollectorResults generates suites and also returns the
// raw collector results, which can be used to discover shared fixtures.
func GenerateSuitesWithCollectorResults(path string) (gotestgen.GenerateResults, []gotestgen.CollectorResult, error) {
	res, crs, err := gotestgen.GenerateWithCollectorResults(path)
	if err != nil {
		return nil, nil, fmt.Errorf("failed generating suites: %w", err)
	}

	return res, crs, nil
}
