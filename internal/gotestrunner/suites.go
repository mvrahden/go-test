package gotestrunner

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/mvrahden/go-test/about"
	"github.com/mvrahden/go-test/internal/cmd/testgen"
	"github.com/mvrahden/go-test/internal/gotestgen"
)

func SuitesGenerate(scanDir string) error {
	results, err := testgen.GenerateSuites(scanDir)
	if err != nil {
		return fmt.Errorf("failed generating suites: %w", err)
	}

	for _, result := range results {
		if len(result.PTest) > 0 {
			testsuiteFile := filepath.Join(result.AbsPath, about.PSuite)
			err := os.WriteFile(testsuiteFile, result.PTest, os.ModePerm)
			if err != nil {
				return fmt.Errorf("failed writing ptest: %w", err)
			}
		}
		if len(result.PXTest) > 0 {
			testsuiteFile := filepath.Join(result.AbsPath, about.PXSuite)
			err := os.WriteFile(testsuiteFile, result.PXTest, os.ModePerm)
			if err != nil {
				return fmt.Errorf("failed writing pxtest: %w", err)
			}
		}
	}
	return nil
}

func SuitesCleanup(pkgPath string) error {
	pkgs, err := gotestgen.LoadCached(pkgPath)
	if err != nil {
		return fmt.Errorf("failed cleaning suites: %w", err)
	}
	if len(pkgs) == 0 {
		return nil
	}
	for _, pkg := range pkgs {
		if pkg.Module == nil {
			continue
		}
		pkgPath := gotestgen.DeterminePkgDir(pkg)
		os.Remove(filepath.Join(pkgPath, about.PSuite))
		os.Remove(filepath.Join(pkgPath, about.PXSuite))
	}
	return nil
}
