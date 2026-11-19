package pkgload

import (
	"context"
	"errors"
	"fmt"
	"go/ast"
	"go/parser"
	"go/scanner"
	"go/token"
	"go/types"
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"sync"

	"golang.org/x/tools/go/gcexportdata"
	"golang.org/x/tools/go/packages"
)

// Config is the part of packages.Config the loads of this module use.
type Config struct {
	Context    context.Context
	Dir        string
	Env        []string
	BuildFlags []string
	Tests      bool
	Fset       *token.FileSet
	ParseFile  func(fset *token.FileSet, filename string, src []byte) (*ast.File, error)
	Overlay    map[string][]byte
}

const (
	// graphMode lists without compiling: any of CompiledGoFiles, Syntax,
	// Types, TypesInfo or TypesSizes makes go list run the build actions
	// of every test variant and test main.
	graphMode = packages.NeedName | packages.NeedFiles | packages.NeedImports | packages.NeedDeps | packages.NeedModule
	// exportMode builds only plain packages, which the test build needs anyway.
	exportMode = packages.NeedName | packages.NeedExportFile | packages.NeedImports | packages.NeedDeps | packages.NeedTypesSizes
	// reportMode is the load a package with errors is reported from.
	reportMode = packages.NeedModule | packages.NeedSyntax | packages.NeedName | packages.NeedTypes | packages.NeedTypesInfo |
		packages.NeedImports | packages.NeedFiles | packages.NeedCompiledGoFiles | packages.NeedTypesSizes
)

// load is packages.Load; tests replace it to see every call.
var load = packages.Load

// Load returns what packages.Load returns for mode
// NeedName|NeedFiles|NeedCompiledGoFiles|NeedImports|NeedModule|NeedSyntax|NeedTypes|NeedTypesInfo|NeedTypesSizes:
// the matched packages (with Tests, their test variants and test mains too), each with
// syntax, types and type information from source and Errors holding what failed.
// It never asks the go command to build a test variant or a test main.
//
// A matched package
// with an error in any of its variants is loaded once more by packages.Load,
// alone, so its errors are the go command's own report.
func Load(cfg *Config, patterns ...string) ([]*packages.Package, error) {
	l := &loader{cfg: cfg, fset: cfg.Fset, files: map[string]*parsed{}}
	if l.fset == nil {
		l.fset = token.NewFileSet()
	}
	if err := l.graph(graphMode, patterns); err != nil {
		return nil, err
	}
	if l.usesCgo() {
		// The files cgo generates are listed only by a compiled listing.
		if err := l.graph(graphMode|packages.NeedCompiledGoFiles, patterns); err != nil {
			return nil, err
		}
	}
	if err := l.exports(); err != nil {
		return nil, err
	}
	if err := l.check(); err != nil {
		return nil, err
	}
	return l.report()
}

type loader struct {
	cfg  *Config
	fset *token.FileSet

	compiled bool                         // the graph lists CompiledGoFiles
	roots    []*packages.Package          // what the patterns matched
	nodes    map[string]*packages.Package // every package reachable from roots, by ID
	source   map[string]*packages.Package // roots and test-rebuilt packages, checked from source
	files    map[string]*parsed           // parsed files by name, shared between variants

	exportFile map[string]string           // plain package path → export data
	exportErrs map[string][]packages.Error // plain package path → errors building it
	sizes      types.Sizes

	tpkgs map[string]*types.Package // by ID; export data fills plain ones in place
	mu    sync.Mutex                // serializes export data reads
}

type parsed struct {
	f   *ast.File
	err error
}

func (l *loader) config(mode packages.LoadMode, tests bool) *packages.Config {
	return &packages.Config{
		Context:    l.cfg.Context,
		Dir:        l.cfg.Dir,
		Env:        l.cfg.Env,
		BuildFlags: l.cfg.BuildFlags,
		Mode:       mode,
		Tests:      tests,
		Fset:       l.fset,
		ParseFile:  l.cfg.ParseFile,
		Overlay:    l.cfg.Overlay,
	}
}

// graph lists the packages and parses the ones checked from source.
func (l *loader) graph(mode packages.LoadMode, patterns []string) error {
	roots, err := load(l.config(mode, l.cfg.Tests), patterns...)
	if err != nil {
		return err
	}
	l.compiled = mode&packages.NeedCompiledGoFiles != 0
	l.roots = roots
	l.nodes = map[string]*packages.Package{}
	l.source = map[string]*packages.Package{}
	packages.Visit(roots, nil, func(p *packages.Package) { l.nodes[p.ID] = p })
	for _, p := range roots {
		l.source[p.ID] = p
	}
	for id, p := range l.nodes {
		if strings.Contains(id, " [") {
			l.source[id] = p
		}
	}
	l.parse()
	return nil
}

// isTestMain reports whether p is the generated main of a test binary.
func isTestMain(p *packages.Package) bool {
	return strings.HasSuffix(p.ID, ".test")
}

// parse parses every file of the source packages once, concurrently.
func (l *loader) parse() {
	var names []string
	for _, p := range l.source {
		if !l.compiled {
			// Without cgo, go list compiles exactly the Go files; a test
			// main's is a build cache file, without an extension.
			p.CompiledGoFiles = nil
			for _, f := range p.GoFiles {
				if ext := filepath.Ext(f); ext == ".go" || ext == "" {
					p.CompiledGoFiles = append(p.CompiledGoFiles, f)
				}
			}
		}
		for _, name := range p.CompiledGoFiles {
			if l.files[name] == nil {
				l.files[name] = &parsed{}
				names = append(names, name)
			}
		}
	}
	parallel(len(names), func(i int) {
		pf := l.files[names[i]]
		pf.f, pf.err = l.parseFile(names[i])
	})
	for _, p := range l.source {
		p.Fset = l.fset
		p.Syntax = nil
		for _, name := range p.CompiledGoFiles {
			pf := l.files[name]
			if pf.err != nil {
				p.Errors = append(p.Errors, parseErrors(pf.err)...)
			}
			if pf.f != nil {
				p.Syntax = append(p.Syntax, pf.f)
			}
		}
	}
}

func (l *loader) parseFile(name string) (*ast.File, error) {
	src, ok := l.cfg.Overlay[name]
	if !ok {
		var err error
		if src, err = os.ReadFile(name); err != nil {
			return nil, err
		}
	}
	if l.cfg.ParseFile != nil {
		return l.cfg.ParseFile(l.fset, name, src)
	}
	return parser.ParseFile(l.fset, name, src, parser.AllErrors|parser.ParseComments)
}

// parseErrors converts a parse failure as go/packages does.
func parseErrors(err error) []packages.Error {
	var list scanner.ErrorList
	var pathErr *os.PathError
	switch {
	case errors.As(err, &list):
		out := make([]packages.Error, len(list))
		for i, e := range list {
			out[i] = packages.Error{Pos: e.Pos.String(), Msg: e.Msg, Kind: packages.ParseError}
		}
		return out
	case errors.As(err, &pathErr):
		return []packages.Error{{Pos: pathErr.Path + ":1", Msg: pathErr.Err.Error(), Kind: packages.ParseError}}
	}
	return []packages.Error{{Pos: "-", Msg: err.Error(), Kind: packages.UnknownError}}
}

// usesCgo reports whether a source package was parsed from files that
// still import "C", so the graph must be listed compiled.
func (l *loader) usesCgo() bool {
	if l.compiled {
		return false
	}
	for _, p := range l.source {
		for _, f := range p.Syntax {
			for _, imp := range f.Imports {
				if imp.Path.Value == `"C"` {
					return true
				}
			}
		}
	}
	return false
}

// exports lists the export data of the plain packages the source packages
// import, and with it the type sizes.
func (l *loader) exports() error {
	seen := map[string]bool{}
	var paths []string
	for _, p := range l.source {
		for _, dep := range p.Imports {
			if l.source[dep.ID] == nil && dep.PkgPath != "unsafe" && dep.PkgPath != "C" && !seen[dep.PkgPath] {
				seen[dep.PkgPath] = true
				paths = append(paths, dep.PkgPath)
			}
		}
	}
	slices.Sort(paths)
	if len(paths) == 0 {
		paths = []string{"unsafe"} // the listing still gives the type sizes
	}
	pkgs, err := load(l.config(exportMode, false), paths...)
	if err != nil {
		return err
	}
	l.exportFile = map[string]string{}
	l.exportErrs = map[string][]packages.Error{}
	packages.Visit(pkgs, nil, func(p *packages.Package) {
		l.exportFile[p.PkgPath] = p.ExportFile
		if len(p.Errors) > 0 {
			l.exportErrs[p.PkgPath] = p.Errors
		}
		if l.sizes == nil {
			l.sizes = p.TypesSizes
		}
	})
	if l.sizes == nil {
		l.sizes = types.SizesFor("gc", runtime.GOARCH)
	}
	return nil
}

// check type-checks the source packages, each once its imports are done.
func (l *loader) check() error {
	ctx := l.cfg.Context
	if ctx == nil {
		ctx = context.Background()
	}
	l.tpkgs = make(map[string]*types.Package, len(l.nodes))
	done := make(map[string]chan struct{}, len(l.nodes))
	for id, p := range l.nodes {
		l.tpkgs[id] = types.NewPackage(p.PkgPath, p.Name)
		if p.PkgPath == "unsafe" {
			l.tpkgs[id] = types.Unsafe
		}
		done[id] = make(chan struct{})
	}
	cpu := make(chan struct{}, runtime.GOMAXPROCS(0))
	var wg sync.WaitGroup
	for id, p := range l.nodes {
		wg.Add(1)
		go func() {
			defer wg.Done()
			defer close(done[id])
			for _, dep := range p.Imports {
				<-done[dep.ID]
			}
			if l.source[id] == nil || ctx.Err() != nil {
				return
			}
			cpu <- struct{}{}
			defer func() { <-cpu }()
			l.checkOne(p)
		}()
	}
	wg.Wait()
	if err := ctx.Err(); err != nil {
		return err
	}
	for id, p := range l.nodes {
		if l.source[id] == nil && l.tpkgs[id].Complete() {
			p.Types = l.tpkgs[id]
		}
	}
	return nil
}

func (l *loader) checkOne(p *packages.Package) {
	p.TypesSizes = l.sizes
	p.TypesInfo = &types.Info{
		Types:        map[ast.Expr]types.TypeAndValue{},
		Defs:         map[*ast.Ident]types.Object{},
		Uses:         map[*ast.Ident]types.Object{},
		Implicits:    map[ast.Node]types.Object{},
		Instances:    map[*ast.Ident]types.Instance{},
		Scopes:       map[ast.Node]*types.Scope{},
		Selections:   map[*ast.SelectorExpr]*types.Selection{},
		FileVersions: map[*ast.File]string{},
	}
	conf := &types.Config{
		Importer: importerFunc(func(path string) (*types.Package, error) { return l.importFor(p, path) }),
		Sizes:    l.sizes,
		Error: func(err error) {
			var te types.Error
			if errors.As(err, &te) {
				p.TypeErrors = append(p.TypeErrors, te)
				p.Errors = append(p.Errors, packages.Error{Pos: te.Fset.Position(te.Pos).String(), Msg: te.Msg, Kind: packages.TypeError})
				return
			}
			p.Errors = append(p.Errors, packages.Error{Pos: "-", Msg: err.Error(), Kind: packages.UnknownError})
		},
	}
	if p.Module != nil && p.Module.GoVersion != "" {
		conf.GoVersion = "go" + p.Module.GoVersion
	}
	tpkg := l.tpkgs[p.ID]
	if err := types.NewChecker(conf, l.fset, tpkg, p.TypesInfo).Files(p.Syntax); err != nil && len(p.Errors) == 0 {
		conf.Error(err)
	}
	p.Types = tpkg
}

type importerFunc func(path string) (*types.Package, error)

func (f importerFunc) Import(path string) (*types.Package, error) { return f(path) }

// importFor resolves an import of p: a package checked from source by its
// own types, any other from export data.
func (l *loader) importFor(p *packages.Package, path string) (*types.Package, error) {
	if path == "unsafe" {
		return types.Unsafe, nil
	}
	dep := p.Imports[path]
	if dep == nil {
		return nil, fmt.Errorf("no metadata for %s", path)
	}
	if l.source[dep.ID] != nil {
		return l.tpkgs[dep.ID], nil
	}
	return l.importExport(dep)
}

// importExport reads dep's export data into the package created for it.
// The view holds dep and everything it reaches, so the reader fills those
// packages in place and every importer sees one types.Package per ID.
func (l *loader) importExport(dep *packages.Package) (*types.Package, error) {
	l.mu.Lock()
	defer l.mu.Unlock()
	if tpkg := l.tpkgs[dep.ID]; tpkg.Complete() {
		return tpkg, nil
	}
	file := l.exportFile[dep.PkgPath]
	if file == "" {
		return nil, errors.New("no export data file")
	}
	f, err := os.Open(file)
	if err != nil {
		return nil, err
	}
	defer func() { _ = f.Close() }()
	r, err := gcexportdata.NewReader(f) //nolint:staticcheck // SA1019: go list -export names compiler archives, as go/packages reads them
	if err != nil {
		return nil, fmt.Errorf("reading %s: %w", file, err)
	}
	view := map[string]*types.Package{}
	var visit func(p *packages.Package)
	visit = func(p *packages.Package) {
		if _, ok := view[p.PkgPath]; ok {
			return
		}
		view[p.PkgPath] = l.tpkgs[p.ID]
		for _, imp := range p.Imports {
			visit(imp)
		}
	}
	visit(dep)
	tpkg, err := gcexportdata.Read(r, l.fset, view, dep.PkgPath)
	if err != nil {
		return nil, fmt.Errorf("reading %s: %w", file, err)
	}
	return tpkg, nil
}

// report returns the roots, with each matched package that has an error
// in any of its variants, or in anything they import, loaded once more:
// the go command's build of it reports the compiler's own words.
func (l *loader) report() ([]*packages.Package, error) {
	memo := map[string]bool{}
	seen := map[string]bool{}
	var broken []string
	for _, p := range l.roots {
		if isTestMain(p) || len(p.GoFiles) == 0 || !l.failed(p, memo) {
			continue // a pattern with no files builds nothing to report
		}
		if path := strings.TrimSuffix(p.PkgPath, "_test"); !seen[path] {
			seen[path] = true
			broken = append(broken, path)
		}
	}
	for id, p := range l.source {
		p.IllTyped = l.failed(l.nodes[id], memo)
	}
	if len(broken) == 0 {
		return l.roots, nil
	}
	slices.Sort(broken)
	reported, err := load(l.config(reportMode, l.cfg.Tests), broken...)
	if err != nil {
		return nil, err
	}
	out := slices.Clone(l.roots)
	at := make(map[string]int, len(out))
	for i, p := range out {
		at[p.ID] = i
	}
	for _, p := range reported {
		if i, ok := at[p.ID]; ok {
			out[i] = p
		} else {
			out = append(out, p)
		}
	}
	return out, nil
}

// failed reports whether p, or anything it imports, has an error.
func (l *loader) failed(p *packages.Package, memo map[string]bool) bool {
	if v, ok := memo[p.ID]; ok {
		return v
	}
	memo[p.ID] = false // the graph is acyclic; this only guards a broken one
	bad := len(p.Errors) > 0 || l.source[p.ID] == nil && len(l.exportErrs[p.PkgPath]) > 0
	for _, dep := range p.Imports {
		if bad {
			break
		}
		bad = l.failed(dep, memo)
	}
	memo[p.ID] = bad
	return bad
}

// parallel runs f(0..n-1) on up to GOMAXPROCS goroutines.
func parallel(n int, f func(i int)) {
	work := make(chan int)
	var wg sync.WaitGroup
	for range min(n, runtime.GOMAXPROCS(0)) {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for i := range work {
				f(i)
			}
		}()
	}
	for i := range n {
		work <- i
	}
	close(work)
	wg.Wait()
}
