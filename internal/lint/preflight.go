package lint

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"maps"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strconv"
	"strings"

	"github.com/mvrahden/go-test/internal/genstub"
	"github.com/mvrahden/go-test/internal/goversion"
	"github.com/mvrahden/go-test/internal/pkgpattern"
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
	// Generated files on disk load as their package clause.
	if t.Tests {
		if stubs := genstub.Stubs(t.Dir, t.Patterns, t.BuildFlags); len(stubs) > 0 {
			maps.Copy(stubs, overlay)
			overlay = stubs
		}
	}
	var replaced map[string]string // overlay file → the file it stands for
	if len(overlay) > 0 {
		cleanup, written, err := withOverlay(cfg, t.TempDir, overlay)
		if err != nil {
			return nil, fmt.Errorf("load packages: %w", err)
		}
		defer cleanup()
		replaced = written
	}
	pkgs, err := packages.Load(cfg, pkgpattern.Clean(t.Patterns)...)
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

	cwd := t.Dir
	if cwd == "" {
		cwd, _ = os.Getwd()
	}
	ce := &compileError{}
	roots := map[*packages.Package]bool{}
	for _, p := range pkgs {
		roots[p] = true
	}
	byPath := map[string]*brokenPackage{}
	seen := map[string]bool{}
	// A dependency that does not compile is named too: its dependents go
	// unanalyzed for it.
	packages.Visit(pkgs, nil, func(p *packages.Package) {
		if len(p.Errors) == 0 {
			return
		}
		if roots[p] && absent(p) {
			// Said as go says it: the path names it.
			for _, e := range p.Errors {
				if !seen[e.Msg] {
					seen[e.Msg] = true
					ce.missing = append(ce.missing, e.Msg)
				}
			}
			return
		}
		b := byPath[p.PkgPath]
		if b == nil {
			b = &brokenPackage{path: p.PkgPath, seen: map[string]bool{}}
			byPath[p.PkgPath] = b
		}
		r := &errorPaths{cwd: cwd, module: cwd, replaced: replaced}
		if p.Module != nil && p.Module.Dir != "" {
			r.module = p.Module.Dir
		}
		for _, line := range packageErrors(p, r) {
			if !b.seen[line] {
				b.seen[line] = true
				b.errs = append(b.errs, line)
			}
		}
	})
	for _, path := range slices.Sorted(maps.Keys(byPath)) {
		if b := byPath[path]; len(b.errs) > 0 {
			ce.broken = append(ce.broken, *b)
		}
	}
	if len(ce.broken) > 0 {
		return nil, ce
	}
	if len(ce.missing) > 0 {
		return nil, errors.New(strings.Join(ce.missing, "\n"))
	}
	return pkgs, nil
}

// compileError names the targets that do not compile, and the patterns
// that name nothing.
type compileError struct {
	missing []string
	broken  []brokenPackage
}

type brokenPackage struct {
	path string
	errs []string // "file:line:col: message", file relative to the module
	seen map[string]bool
}

func (e *compileError) Error() string {
	lines := slices.Clone(e.missing)
	var broken []string
	for _, b := range e.broken {
		broken = append(broken, b.path+"\n\t"+strings.Join(b.errs, "\n\t"))
	}
	lines = append(lines, "cannot lint uncompilable packages — nothing was proven about them:\n"+strings.Join(broken, "\n"))
	return strings.Join(lines, "\n")
}

// firstMessage is the first compiler error, without its position. A
// package is only listed as broken with at least one error.
func (e *compileError) firstMessage() string {
	if len(e.broken) == 0 {
		return ""
	}
	line := e.broken[0].errs[0]
	if m := positioned.FindStringSubmatch(line); m != nil {
		return m[2]
	}
	return line
}

// positioned matches "file:line[:col]: message".
var positioned = regexp.MustCompile(`^(.+?:\d+(?::\d+)?): (.*)$`)

// errorPaths renders the files errors name relative to the module.
type errorPaths struct {
	cwd, module string
	replaced    map[string]string
}

func (r *errorPaths) rel(file string) string {
	if orig, ok := r.replaced[file]; ok {
		file = orig
	}
	if !filepath.IsAbs(file) {
		file = filepath.Join(r.cwd, file)
	}
	if rel, err := filepath.Rel(r.module, file); err == nil && !strings.HasPrefix(rel, "..") {
		return filepath.ToSlash(rel)
	}
	return file
}

// position renders "file:line[:col]" with the file relative to the module.
func (r *errorPaths) position(pos string) string {
	m := filePosition.FindStringSubmatch(pos)
	if m == nil {
		return pos
	}
	return r.rel(m[1]) + m[2]
}

var filePosition = regexp.MustCompile(`^(.+?)(:\d+(?::\d+)?)$`)

// packageErrors renders p's errors one line each. The type checker and the
// go command's build of the export data report the same compiler errors;
// the build's are read only when the type checker reported none.
func packageErrors(p *packages.Package, r *errorPaths) []string {
	checked := false
	for _, e := range p.Errors {
		if e.Kind == packages.TypeError || e.Kind == packages.ParseError {
			checked = true
		}
	}
	var out []string
	for _, e := range p.Errors {
		if build, ok := strings.CutPrefix(e.Msg, "# "); ok && e.Kind == packages.ListError {
			if checked {
				continue
			}
			for _, line := range strings.Split(build, "\n")[1:] {
				if m := positioned.FindStringSubmatch(line); m != nil {
					out = append(out, r.position(m[1])+": "+m[2])
				} else if line = strings.TrimSpace(line); line != "" {
					out = append(out, line)
				}
			}
			continue
		}
		if e.Pos == "" || e.Pos == "-" {
			out = append(out, e.Msg)
			continue
		}
		out = append(out, r.position(e.Pos)+": "+e.Msg)
	}
	return out
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
func withOverlay(cfg *packages.Config, tmp string, overlay map[string][]byte) (cleanup func(), written map[string]string, err error) {
	dir, err := os.MkdirTemp(tmp, "gotest-lint-overlay")
	if err != nil {
		return nil, nil, err
	}
	cleanup = func() { _ = os.RemoveAll(dir) }
	replace := map[string]string{}
	written = map[string]string{}
	i := 0
	for file, src := range overlay {
		i++
		path := filepath.Join(dir, strconv.Itoa(i)+"-"+filepath.Base(file))
		if err := os.WriteFile(path, src, 0o600); err != nil {
			cleanup()
			return nil, nil, err
		}
		replace[file] = path
		written[path] = file
	}
	data, err := json.Marshal(struct{ Replace map[string]string }{replace})
	if err == nil {
		err = os.WriteFile(filepath.Join(dir, "overlay.json"), data, 0o600)
	}
	if err != nil {
		cleanup()
		return nil, nil, err
	}
	cfg.BuildFlags = append(cfg.BuildFlags, "-overlay="+filepath.Join(dir, "overlay.json"))
	// go/packages reads the disk; parse the overlay instead, as it would.
	cfg.ParseFile = func(fset *token.FileSet, filename string, src []byte) (*ast.File, error) {
		if o, ok := overlay[filename]; ok {
			src = o
		}
		return parser.ParseFile(fset, filename, src, parser.AllErrors|parser.ParseComments)
	}
	return cleanup, written, nil
}
