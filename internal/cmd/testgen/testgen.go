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
