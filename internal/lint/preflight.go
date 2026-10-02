package lint

import (
	"fmt"
	"strings"

	"github.com/mvrahden/go-test/internal/goversion"
	"golang.org/x/tools/go/packages"
)

// PreflightLoad type-checks the lint targets before the go/analysis driver
// runs, for the invocations that still go through it. dir is the module
// directory to load from; "" means the current directory, matching the
// driver's own resolution.
//
// The driver prints "analysis skipped due to errors in package" for an
// uncompilable package and still exits 0 — a skipped analysis reported as a
// pass. Nothing was proven about that package, so it has to fail loudly
// instead; this is the same success-is-proven contract the runner applies to
// compile failures.
func PreflightLoad(dir string, patterns []string) error {
	_, err := load(dir, patterns)
	return err
}

// load type-checks the targets from source and reads their dependencies
// from export data: the analyzer holds no facts, so it never looks at a
// dependency's syntax. Every target that does not compile is named in the
// error.
func load(dir string, patterns []string) ([]*packages.Package, error) {
	cfg := &packages.Config{
		Dir:   dir,
		Mode:  packages.LoadSyntax | packages.NeedModule,
		Tests: true,
	}
	pkgs, err := packages.Load(cfg, patterns...)
	if err != nil {
		return nil, fmt.Errorf("load packages: %w", err)
	}
	if err := goversion.Check(pkgs); err != nil {
		return nil, err
	}

	var broken []string
	seen := map[string]bool{}
	for _, p := range pkgs {
		if len(p.Errors) == 0 || seen[p.PkgPath] {
			continue
		}
		seen[p.PkgPath] = true
		msgs := make([]string, 0, len(p.Errors))
		for _, e := range p.Errors {
			msgs = append(msgs, "\t"+e.Error())
		}
		broken = append(broken, p.PkgPath+"\n"+strings.Join(msgs, "\n"))
	}
	if len(broken) > 0 {
		return nil, fmt.Errorf("cannot lint uncompilable packages — nothing was proven about them:\n%s", strings.Join(broken, "\n"))
	}
	return pkgs, nil
}

// PreflightPatterns extracts the package patterns from a mixed flag/pattern
// argument list, for drivers that hand the whole argv to the analysis
// framework.
func PreflightPatterns(args []string) []string {
	var patterns []string
	for _, a := range args {
		if !strings.HasPrefix(a, "-") {
			patterns = append(patterns, a)
		}
	}
	return patterns
}
