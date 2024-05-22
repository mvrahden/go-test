package gotestgen

import (
	"fmt"
	"strings"

	"github.com/mvrahden/go-test/internal/x/slices"
	"golang.org/x/tools/go/packages"
)

const (
	packageEvalMode = packages.NeedSyntax | packages.NeedName | packages.NeedTypes | packages.NeedTypesInfo
)

func Generate(targetPath string) (string, []byte, error) {
	pkgName, srcs, err := generateFile(targetPath)
	if err != nil {
		return "", nil, err
	}
	return pkgName, srcs, nil
}

func loadPackages(targetPkg string) ([]*packages.Package, error) {
	p, err := packages.Load(&packages.Config{
		Mode:  packageEvalMode,
		Tests: true,
	}, targetPkg)
	if err != nil {
		return nil, fmt.Errorf("failed loading packages. err: %w", err)
	}

	// filter all test-related packages
	p = slices.Filter(p, func(item *packages.Package, index int) bool {
		return strings.HasSuffix(item.ID, ".test]")
	})
	if len(p) == 0 {
		return nil, fmt.Errorf("no test files found")
	}
	if len(p) > 2 {
		return nil, fmt.Errorf("loaded unexpected amount of packages. want: n <= 2, got: %d", len(p))
	}
	return p, nil
}

func generateFile(targetPkg string) (string, []byte, error) {
	pkgs, err := loadPackages(targetPkg)
	if err != nil {
		return "", nil, err
	}
	c := collector{}
	result := c.CollectSuiteSpecs(pkgs)
	if len(result.Errs) > 0 {
		return "", nil, result.Errs[0].Err
	}

	out, err := c.ApplyGoTestSpecs(result.Suites)
	if err != nil {
		return "", nil, err
	}

	r := renderer{}
	buf, err := r.RenderGoTestSpec(pkgs, out)
	if err != nil {
		return "", nil, err
	}
	return strings.TrimSuffix(pkgs[0].Name, "_test"), buf, nil
}
