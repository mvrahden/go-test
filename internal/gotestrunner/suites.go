package gotestrunner

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"github.com/mvrahden/go-test/about"
	"github.com/mvrahden/go-test/internal/cmd/testgen"
	"golang.org/x/tools/go/packages"
)

func SuitesGenerate(scanDir string) error {
	result, err := testgen.GenerateSuites(scanDir)
	if err != nil {
		return fmt.Errorf("failed generating suites: %w", err)
	}

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
	return nil
}

func SuitesCleanup(pkgPath string) error {
	pkgs, err := packages.Load(&packages.Config{
		Mode: packages.NeedModule,
	}, pkgPath)
	if err != nil {
		return fmt.Errorf("failed cleaning suites: %w", err)
	}
	if len(pkgs) == 0 {
		return nil
	}
	modDir := pkgs[0].Module.Dir
	fs.WalkDir(os.DirFS(modDir), ".", func(path string, d fs.DirEntry, err error) error {
		if d != nil && d.IsDir() {
			return nil
		}
		if strings.HasPrefix(path, ".git") {
			return nil
		}
		if !strings.HasSuffix(path, "_test.go") {
			return nil
		}
		if !about.PSuiteRegex.MatchString(path) {
			return nil
		}
		detected := filepath.Join(modDir, path)
		os.Remove(detected)
		return nil
	})
	return nil
}
