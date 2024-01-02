package gotestgen

import (
	"errors"
	"fmt"
	"strings"

	"github.com/samber/lo"
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
	return errors.New("not implemented yet")
}

func loadPackage(targetPkg string) ([]*packages.Package, error) {
	p, err := packages.Load(&packages.Config{
		Mode:  packageEvalMode,
		Tests: true,
	}, targetPkg)
	if err != nil {
		return nil, fmt.Errorf("failed loading packages. err: %w", err)
	}
	p = lo.Filter(p, func(item *packages.Package, index int) bool {
		return strings.HasSuffix(item.PkgPath, ".test") || strings.HasSuffix(item.PkgPath, "_test")
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
	pkg, err := loadPackage(targetPkg)
	if err != nil {
		return nil, err
	}
	c := collector{}
	specs, err := c.CollectSuiteSpecs(pkg)
	fmt.Printf("specs: %+v\n", specs)
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
	return nil, nil
}
