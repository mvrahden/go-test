package gotestrunner

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/mvrahden/go-test/about"
	"github.com/mvrahden/go-test/internal/cmd/testgen"
	"github.com/mvrahden/go-test/internal/gotestgen"
)

func SuitesGenerate(pkgPattern string) ([]string, error) {
	results, err := testgen.GenerateSuites(pkgPattern)
	if err != nil {
		return nil, fmt.Errorf("failed generating suites: %w", err)
	}

	dirs, err := writeGeneratedFiles(results)
	if err != nil {
		return dirs, err
	}
	return dirs, nil
}

// SuitesGenerateWithCollectorResults generates test suites and also returns
// the raw collector results, which can be used to discover shared fixtures.
func SuitesGenerateWithCollectorResults(pkgPattern string) ([]string, []gotestgen.CollectorResult, error) {
	results, collectorResults, err := testgen.GenerateSuitesWithCollectorResults(pkgPattern)
	if err != nil {
		return nil, nil, fmt.Errorf("failed generating suites: %w", err)
	}

	dirs, err := writeGeneratedFiles(results)
	if err != nil {
		return dirs, collectorResults, err
	}
	return dirs, collectorResults, nil
}

func writeGeneratedFiles(results gotestgen.GenerateResults) ([]string, error) {
	var dirs []string
	for _, result := range results {
		if len(result.PTest) > 0 {
			testsuiteFile := filepath.Join(result.AbsPath, about.PSuite)
			err := os.WriteFile(testsuiteFile, result.PTest, 0644)
			if err != nil {
				return dirs, fmt.Errorf("failed writing ptest: %w", err)
			}
		}
		if len(result.PXTest) > 0 {
			testsuiteFile := filepath.Join(result.AbsPath, about.PXSuite)
			err := os.WriteFile(testsuiteFile, result.PXTest, 0644)
			if err != nil {
				return dirs, fmt.Errorf("failed writing pxtest: %w", err)
			}
		}
		dirs = append(dirs, result.AbsPath)
	}
	return dirs, nil
}
