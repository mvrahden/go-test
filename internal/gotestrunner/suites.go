package gotestrunner

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/mvrahden/go-test/about"
	"github.com/mvrahden/go-test/internal/cmd/testgen"
)

func SuitesGenerate(pkgPattern string) ([]string, error) {
	results, err := testgen.GenerateSuites(pkgPattern)
	if err != nil {
		return nil, fmt.Errorf("failed generating suites: %w", err)
	}

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
