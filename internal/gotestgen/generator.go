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

func Generate() error {
	srcs, err := generateFile(".")
	if err != nil {
		return err
	}
	fmt.Printf("sources: %s\n", srcs)
	return nil
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
		return nil, fmt.Errorf("no tests or test package found")
	}
	if len(p) > 2 {
		return nil, fmt.Errorf("loaded unexpected amount of packages. want: n <= 2, got: %d", len(p))
	}
	return p, nil
}

func generateFile(targetPkg string) ([]byte, error) {
	pkgs, err := loadPackages(targetPkg)
	if err != nil {
		return nil, err
	}
	c := collector{}
	result := c.CollectSuiteSpecs(pkgs)
	if len(result.Errs) > 0 {
		return nil, result.Errs[0].Err
	}

	out, err := c.ApplyGoTestSpecs(result.Suites)
	if err != nil {
		return nil, err
	}

	r := renderer{}
	buf, err := r.RenderGoTestSpec(pkgs, out)
	if err != nil {
		return nil, err
	}
	// out, err := g.i.Inspect(pkg)
	// if err != nil {
	// 	return nil, err
	// }
	// if len(out.TypeSpecs) == 0 {
	// 	return nil, fmt.Errorf("no enums detected.")
	// }
	// buf, err := g.r.Render(out)
	// if err != nil {
	// 	return nil, err
	// }
	return buf, nil
}
