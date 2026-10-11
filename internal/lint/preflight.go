package lint

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"slices"
	"strconv"
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
	_, err := load(context.Background(), &Targets{Dir: dir, Patterns: patterns, Tests: true}, nil)
	return err
}

// load type-checks the targets from source and reads their dependencies
// from export data: the analyzer holds no facts, so it never looks at a
// dependency's syntax. Every target that does not compile is named in the
// error. overlay replaces the contents of the files it names.
func load(ctx context.Context, t *Targets, overlay map[string][]byte) ([]*packages.Package, error) {
	cfg := &packages.Config{
		Context: ctx,
		Dir:     t.Dir,
		Mode:    packages.LoadSyntax | packages.NeedModule,
		Tests:   t.Tests,
		// withOverlay appends to it.
		BuildFlags: slices.Clone(t.BuildFlags),
	}
	if t.TempDir != "" {
		// A canceled load interrupts go list, which ignores the interrupt
		// and is killed before it removes its work directory.
		cfg.Env = append(os.Environ(), "GOTMPDIR="+t.TempDir)
	}
	if len(overlay) > 0 {
		cleanup, err := withOverlay(cfg, t.TempDir, overlay)
		if err != nil {
			return nil, fmt.Errorf("load packages: %w", err)
		}
		defer cleanup()
	}
	pkgs, err := packages.Load(cfg, t.Patterns...)
	if err != nil {
		return nil, fmt.Errorf("load packages: %w", err)
	}
	// go list failing outright also loads nothing, without an error.
	if len(pkgs) == 0 {
		quoted := make([]string, len(t.Patterns))
		for i, p := range t.Patterns {
			quoted[i] = strconv.Quote(p)
		}
		return nil, fmt.Errorf("%s matched no packages", strings.Join(quoted, ", "))
	}
	if err := goversion.Check(pkgs); err != nil {
		return nil, err
	}

	var missing, broken []string
	seen := map[string]bool{}
	for _, p := range pkgs {
		if len(p.Errors) == 0 || seen[p.PkgPath] {
			continue
		}
		seen[p.PkgPath] = true
		if absent(p) {
			// Said as go says it: the path names it.
			for _, e := range p.Errors {
				missing = append(missing, e.Msg)
			}
			continue
		}
		msgs := make([]string, 0, len(p.Errors))
		for _, e := range p.Errors {
			msgs = append(msgs, "\t"+e.Error())
		}
		broken = append(broken, p.PkgPath+"\n"+strings.Join(msgs, "\n"))
	}
	if len(broken) > 0 {
		missing = append(missing, "cannot lint uncompilable packages — nothing was proven about them:\n"+strings.Join(broken, "\n"))
	}
	if len(missing) > 0 {
		return nil, errors.New(strings.Join(missing, "\n"))
	}
	return pkgs, nil
}

// absent reports whether p is a pattern or path go list found no Go files
// for: a directory or import path that does not exist, or one whose files
// build constraints all exclude. There is nothing to compile, so nothing
// failed to.
func absent(p *packages.Package) bool {
	if len(p.GoFiles) > 0 || len(p.CompiledGoFiles) > 0 {
		return false
	}
	for _, e := range p.Errors {
		if e.Kind != packages.ListError {
			return false
		}
	}
	return true
}

// withOverlay makes cfg load overlay's contents in place of the files it
// names. Config.Overlay would do it too, but go/packages then distrusts all
// export data and type-checks every dependency from source; the go command's
// own -overlay builds export data from the overlay, so it stays usable.
func withOverlay(cfg *packages.Config, tmp string, overlay map[string][]byte) (cleanup func(), err error) {
	dir, err := os.MkdirTemp(tmp, "gotest-lint-overlay")
	if err != nil {
		return nil, err
	}
	cleanup = func() { _ = os.RemoveAll(dir) }
	replace := map[string]string{}
	i := 0
	for file, src := range overlay {
		i++
		path := filepath.Join(dir, strconv.Itoa(i)+"-"+filepath.Base(file))
		if err := os.WriteFile(path, src, 0o600); err != nil {
			cleanup()
			return nil, err
		}
		replace[file] = path
	}
	data, err := json.Marshal(struct{ Replace map[string]string }{replace})
	if err == nil {
		err = os.WriteFile(filepath.Join(dir, "overlay.json"), data, 0o600)
	}
	if err != nil {
		cleanup()
		return nil, err
	}
	cfg.BuildFlags = append(cfg.BuildFlags, "-overlay="+filepath.Join(dir, "overlay.json"))
	// go/packages reads the disk; parse the overlay instead, as it would.
	cfg.ParseFile = func(fset *token.FileSet, filename string, src []byte) (*ast.File, error) {
		if o, ok := overlay[filename]; ok {
			src = o
		}
		return parser.ParseFile(fset, filename, src, parser.AllErrors|parser.ParseComments)
	}
	return cleanup, nil
}
