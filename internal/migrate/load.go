package migrate

import (
	"encoding/json"
	"errors"
	"fmt"
	"go/ast"
	"go/build"
	"go/build/constraint"
	"go/parser"
	"go/scanner"
	"go/token"
	"go/types"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"slices"
	"strconv"
	"strings"
	"sync"

	"golang.org/x/tools/go/packages"
)

// A build configuration is the -tags a directory's test files are
// type-checked with; nil is the default build.
type tagSet []string

func (ts tagSet) key() string { return strings.Join(ts, ",") }

// dirJob is one directory being migrated: its test files, the build
// configurations that together compile them, and the packages each loads.
type dirJob struct {
	dir     string // absolute
	modRoot string
	tests   []string // absolute, sorted
	configs []tagSet // the default first
	// unchecked are test files no configuration compiles, with the
	// constraint that keeps them out.
	unchecked map[string]string
}

// newDirJob plans the configurations that compile dir's test files: the
// default build, plus the tags each file left out of it asks for.
func newDirJob(dir, modRoot string, tests []string) *dirJob {
	j := &dirJob{dir: dir, modRoot: modRoot, tests: tests, configs: []tagSet{nil}, unchecked: map[string]string{}}
	seen := map[string]bool{"": true}
	for _, path := range tests {
		name := filepath.Base(path)
		if ok, err := build.Default.MatchFile(dir, name); err == nil && ok {
			continue
		}
		expr := buildConstraint(path)
		tags := positiveTags(expr)
		ctx := build.Default
		ctx.BuildTags = tags
		if ok, err := ctx.MatchFile(dir, name); err != nil || !ok || len(tags) == 0 {
			j.unchecked[path] = constraintText(expr, name)
			continue
		}
		ts := tagSet(tags)
		if !seen[ts.key()] {
			seen[ts.key()] = true
			j.configs = append(j.configs, ts)
		}
	}
	return j
}

// buildConstraint is the //go:build expression heading path, nil for none.
func buildConstraint(path string) constraint.Expr {
	src, err := os.ReadFile(path)
	if err != nil {
		return nil
	}
	return headerConstraint(src)
}

// headerConstraint is the //go:build expression of src's header.
func headerConstraint(src []byte) constraint.Expr {
	for line := range strings.Lines(string(src)) {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "//") && !constraint.IsGoBuild(line) {
			continue
		}
		if !constraint.IsGoBuild(line) {
			return nil // past the header
		}
		expr, err := constraint.Parse(line)
		if err != nil {
			return nil
		}
		return expr
	}
	return nil
}

// positiveTags are the tags expr asks for, leaving out what the host
// decides: GOOS, GOARCH, compilers, cgo and Go versions.
func positiveTags(expr constraint.Expr) []string {
	set := map[string]bool{}
	var walk func(e constraint.Expr, negated bool)
	walk = func(e constraint.Expr, negated bool) {
		switch x := e.(type) {
		case *constraint.NotExpr:
			walk(x.X, !negated)
		case *constraint.AndExpr:
			walk(x.X, negated)
			walk(x.Y, negated)
		case *constraint.OrExpr:
			walk(x.X, negated)
			walk(x.Y, negated)
		case *constraint.TagExpr:
			if !negated && !hostTag(x.Tag) {
				set[x.Tag] = true
			}
		}
	}
	if expr != nil {
		walk(expr, false)
	}
	tags := make([]string, 0, len(set))
	for tag := range set {
		tags = append(tags, tag)
	}
	slices.Sort(tags)
	return tags
}

func hostTag(tag string) bool {
	switch tag {
	case "cgo", "gc", "gccgo", "unix", "ignore":
		return true
	}
	return strings.HasPrefix(tag, "go1.") || knownOS[tag] || knownArch[tag]
}

var knownOS = setOf("aix", "android", "darwin", "dragonfly", "freebsd", "hurd", "illumos", "ios", "js", "linux", "nacl", "netbsd", "openbsd", "plan9", "solaris", "wasip1", "windows", "zos")

var knownArch = setOf("386", "amd64", "amd64p32", "arm", "arm64", "arm64be", "armbe", "loong64", "mips", "mips64", "mips64le", "mips64p32", "mips64p32le", "mipsle", "ppc", "ppc64", "ppc64le", "riscv", "riscv64", "s390", "s390x", "sparc", "sparc64", "wasm")

func setOf(names ...string) map[string]bool {
	m := make(map[string]bool, len(names))
	for _, n := range names {
		m[n] = true
	}
	return m
}

// constraintText names what keeps a file out of every checked build.
func constraintText(expr constraint.Expr, name string) string {
	if expr != nil {
		return "//go:build " + expr.String()
	}
	return "its file name " + name
}

// loaded is one directory's test packages under one configuration.
type loaded struct {
	tags tagSet
	pkgs []*checkedPkg // the internal and external test packages
}

// checkedPkg is a test package type-checked from source.
type checkedPkg struct {
	id     string
	files  []string // its compiled Go files, as syntax is
	syntax []*ast.File
	types  *types.Package
	info   *types.Info
	errs   []loadErr
}

// loadErr is a package error, positioned when it names a place.
type loadErr struct {
	file string
	line int
	msg  string
}

func (e loadErr) String() string {
	if e.file == "" {
		return e.msg
	}
	return fmt.Sprintf("%s:%d: %s", e.file, e.line, e.msg)
}

// errorsOf lists the errors of ls's packages, each once.
func errorsOf(ls []loaded) []loadErr {
	var out []loadErr
	seen := map[string]bool{}
	for _, l := range ls {
		for _, p := range l.pkgs {
			for _, e := range p.errs {
				if k := e.String(); !seen[k] {
					seen[k] = true
					out = append(out, e)
				}
			}
		}
	}
	return out
}

// errPos matches "file:line:col" or "file:line"; the file may hold
// colons itself (C:\x).
var errPos = regexp.MustCompile(`^(.+?):(\d+)(?::\d+)?$`)

// parseLoadErr splits the position off a package error.
func parseLoadErr(e packages.Error) loadErr {
	if m := errPos.FindStringSubmatch(e.Pos); m != nil {
		line, _ := strconv.Atoi(m[2])
		return loadErr{file: m[1], line: line, msg: e.Msg}
	}
	return loadErr{msg: e.Msg}
}

// loadJobs type-checks the jobs' test packages for real, in every
// configuration of every job: the go command lists them once per module
// and configuration, their dependencies come from export data and the test
// packages themselves are type-checked from source, as go vet does; asking
// go/packages for their types would have the go command compile them too.
// overlay replaces the contents of the files it names; fset and srcs
// receive the syntax and the parsed bytes.
func loadJobs(jobs []*dirJob, overlay map[string][]byte, fset *token.FileSet, srcs map[string][]byte) (map[*dirJob][]loaded, error) {
	type batch struct {
		modRoot string
		tags    tagSet
		jobs    []*dirJob
	}
	var batches []*batch
	byKey := map[string]*batch{}
	for _, j := range jobs {
		for _, ts := range j.configs {
			k := j.modRoot + "\x00" + ts.key()
			b := byKey[k]
			if b == nil {
				b = &batch{modRoot: j.modRoot, tags: ts}
				byKey[k] = b
				batches = append(batches, b)
			}
			b.jobs = append(b.jobs, j)
		}
	}
	cleanup, overlayFlag, err := writeOverlay(overlay)
	if err != nil {
		return nil, err
	}
	defer cleanup()

	out := map[*dirJob][]loaded{}
	for _, b := range batches {
		flags := []string{modFlag(b.modRoot)}
		if len(b.tags) > 0 {
			flags = append(flags, "-tags="+b.tags.key())
		}
		patterns := make([]string, 0, len(b.jobs))
		for _, j := range b.jobs {
			rel, err := filepath.Rel(b.modRoot, j.dir)
			if err != nil {
				return nil, err
			}
			patterns = append(patterns, "./"+filepath.ToSlash(rel))
		}
		listFlags := flags
		if overlayFlag != "" {
			listFlags = append(slices.Clone(flags), overlayFlag)
		}
		meta, err := packages.Load(&packages.Config{
			Dir:        b.modRoot,
			Mode:       packages.NeedName | packages.NeedFiles | packages.NeedCompiledGoFiles | packages.NeedImports | packages.NeedModule,
			Tests:      true,
			BuildFlags: listFlags,
		}, patterns...)
		if err != nil {
			return nil, fmt.Errorf("load %s: %w", strings.Join(patterns, " "), err)
		}
		roots := map[*dirJob][]*packages.Package{}
		isRoot := map[string]bool{}
		for _, j := range b.jobs {
			roots[j] = testPackagesOf(meta, j)
			for _, p := range roots[j] {
				isRoot[p.ID] = true
			}
		}
		ids, variant := depIDs(roots, isRoot)
		if variant {
			// A dependency recompiled against a test package (it imports
			// the package back) shares that package's identity, which only
			// go/packages' own loader keeps; it compiles the test packages.
			if err := loadWhole(b.modRoot, listFlags, patterns, b.jobs, b.tags, overlay, fset, srcs, out); err != nil {
				return nil, err
			}
			continue
		}
		deps, err := loadDeps(b.modRoot, flags, ids)
		if err != nil {
			return nil, err
		}
		for _, j := range b.jobs {
			pkgs := checkRoots(roots[j], deps, overlay, fset, srcs)
			out[j] = append(out[j], loaded{tags: b.tags, pkgs: pkgs})
		}
	}
	return out, nil
}

// modFlag keeps the go command from writing go.mod or go.sum, whatever
// GOFLAGS asks for: a module with a vendor directory loads from it, as the
// go command would, unless a workspace governs it.
func modFlag(modRoot string) string {
	if _, err := os.Stat(filepath.Join(modRoot, "vendor", "modules.txt")); err == nil && workspaceFile(modRoot) == "" {
		return "-mod=vendor"
	}
	return "-mod=readonly"
}

// workspaceFile is the go.work that governs dir, empty when none does.
func workspaceFile(dir string) string {
	switch env := os.Getenv("GOWORK"); env {
	case "off":
		return ""
	case "":
	default:
		return env
	}
	for d := dir; ; {
		if _, err := os.Stat(filepath.Join(d, "go.work")); err == nil {
			return filepath.Join(d, "go.work")
		}
		parent := filepath.Dir(d)
		if parent == d {
			return ""
		}
		d = parent
	}
}

// depIDs lists what the roots import; variant reports a dependency
// recompiled for a test.
func depIDs(roots map[*dirJob][]*packages.Package, isRoot map[string]bool) (ids []string, variant bool) {
	seen := map[string]bool{}
	for _, ps := range roots {
		for _, p := range ps {
			for _, imp := range p.Imports {
				if isRoot[imp.ID] || seen[imp.ID] || imp.ID == "unsafe" || imp.ID == "C" {
					continue
				}
				seen[imp.ID] = true
				ids = append(ids, imp.ID)
				variant = variant || strings.Contains(imp.ID, " [")
			}
		}
	}
	return ids, variant
}

// loadWhole loads the test packages with go/packages alone: syntax and
// types for them, export data for their dependencies.
func loadWhole(modRoot string, flags, patterns []string, jobs []*dirJob, tags tagSet, overlay map[string][]byte, fset *token.FileSet, srcs map[string][]byte, out map[*dirJob][]loaded) error {
	pkgs, err := packages.Load(&packages.Config{
		Dir:        modRoot,
		Mode:       packages.LoadSyntax | packages.NeedModule,
		Tests:      true,
		Fset:       fset,
		BuildFlags: flags,
		ParseFile: func(fs *token.FileSet, filename string, src []byte) (*ast.File, error) {
			if o, ok := overlay[filename]; ok {
				src = o
			}
			if srcs != nil {
				srcMu.Lock()
				srcs[filename] = src
				srcMu.Unlock()
			}
			return parser.ParseFile(fs, filename, src, parser.AllErrors|parser.ParseComments|parser.SkipObjectResolution)
		},
	}, patterns...)
	if err != nil {
		return fmt.Errorf("load %s: %w", strings.Join(patterns, " "), err)
	}
	for _, j := range jobs {
		var cps []*checkedPkg
		for _, p := range testPackagesOf(pkgs, j) {
			cp := &checkedPkg{id: p.ID, files: p.CompiledGoFiles, syntax: p.Syntax, types: p.Types, info: p.TypesInfo}
			for _, e := range p.Errors {
				if e.Kind == packages.ListError && strings.HasPrefix(e.Msg, "# ") && len(p.TypeErrors) > 0 {
					continue // the compiler's report of the type errors listed too
				}
				cp.errs = append(cp.errs, parseLoadErr(e))
			}
			cps = append(cps, cp)
		}
		out[j] = append(out[j], loaded{tags: tags, pkgs: cps})
	}
	return nil
}

var srcMu sync.Mutex

// loadDeps loads the packages ids name, from export data.
func loadDeps(modRoot string, flags, ids []string) (map[string]*packages.Package, error) {
	deps := map[string]*packages.Package{}
	if len(ids) == 0 {
		return deps, nil
	}
	pkgs, err := packages.Load(&packages.Config{
		Dir:        modRoot,
		Mode:       packages.NeedName | packages.NeedTypes | packages.NeedImports,
		BuildFlags: flags,
	}, ids...)
	if err != nil {
		return nil, fmt.Errorf("load dependencies: %w", err)
	}
	packages.Visit(pkgs, nil, func(p *packages.Package) {
		deps[p.ID] = p
	})
	return deps, nil
}

// checkRoots type-checks a directory's test packages from source: the
// internal one first, which the external one may import.
func checkRoots(roots []*packages.Package, deps map[string]*packages.Package, overlay map[string][]byte, fset *token.FileSet, srcs map[string][]byte) []*checkedPkg {
	slices.SortStableFunc(roots, func(a, b *packages.Package) int {
		return cmpBool(strings.HasSuffix(a.Name, "_test"), strings.HasSuffix(b.Name, "_test"))
	})
	checked := map[string]*types.Package{}
	out := make([]*checkedPkg, 0, len(roots))
	for _, p := range roots {
		cp := &checkedPkg{id: p.ID, files: p.CompiledGoFiles}
		for _, e := range p.Errors {
			cp.errs = append(cp.errs, parseLoadErr(e))
		}
		for _, file := range p.CompiledGoFiles {
			src, ok := overlay[file]
			if !ok {
				var err error
				if src, err = os.ReadFile(file); err != nil {
					cp.errs = append(cp.errs, loadErr{file: file, msg: err.Error()})
					continue
				}
			}
			if srcs != nil {
				srcs[file] = src
			}
			f, err := parser.ParseFile(fset, file, src, parser.AllErrors|parser.ParseComments|parser.SkipObjectResolution)
			var list scanner.ErrorList
			if errors.As(err, &list) {
				for _, e := range list {
					cp.errs = append(cp.errs, loadErr{file: e.Pos.Filename, line: e.Pos.Line, msg: e.Msg})
				}
			}
			if f != nil {
				cp.syntax = append(cp.syntax, f)
			}
		}
		cp.info = &types.Info{
			Types:        map[ast.Expr]types.TypeAndValue{},
			Defs:         map[*ast.Ident]types.Object{},
			Uses:         map[*ast.Ident]types.Object{},
			Implicits:    map[ast.Node]types.Object{},
			Selections:   map[*ast.SelectorExpr]*types.Selection{},
			Scopes:       map[ast.Node]*types.Scope{},
			Instances:    map[*ast.Ident]types.Instance{},
			FileVersions: map[*ast.File]string{},
		}
		conf := types.Config{
			Importer: importerFunc(func(path string) (*types.Package, error) {
				if path == "unsafe" {
					return types.Unsafe, nil
				}
				imp := p.Imports[path]
				if imp == nil {
					return nil, fmt.Errorf("%s is not among the imports the go command resolved", path)
				}
				if t := checked[imp.ID]; t != nil {
					return t, nil
				}
				dep := deps[imp.ID]
				if dep == nil || dep.Types == nil || dep.Types.Name() == "" || !dep.Types.Complete() {
					msgs := []string{"no export data"}
					if dep != nil && len(dep.Errors) > 0 {
						msgs = msgs[:0]
						for _, e := range dep.Errors {
							msgs = append(msgs, e.Msg)
						}
					}
					return nil, errors.New(strings.Join(msgs, "; "))
				}
				return dep.Types, nil
			}),
			Error: func(err error) {
				var te types.Error
				if errors.As(err, &te) {
					pos := te.Fset.Position(te.Pos)
					cp.errs = append(cp.errs, loadErr{file: pos.Filename, line: pos.Line, msg: te.Msg})
					return
				}
				cp.errs = append(cp.errs, loadErr{msg: err.Error()})
			},
			Sizes: types.SizesFor("gc", runtime.GOARCH),
		}
		if p.Module != nil && p.Module.GoVersion != "" {
			conf.GoVersion = "go" + p.Module.GoVersion
		}
		cp.types, _ = conf.Check(p.PkgPath, fset, cp.syntax, cp.info)
		checked[p.ID] = cp.types
		out = append(out, cp)
	}
	return out
}

type importerFunc func(path string) (*types.Package, error)

func (f importerFunc) Import(path string) (*types.Package, error) { return f(path) }

func cmpBool(a, b bool) int {
	switch {
	case a == b:
		return 0
	case !a:
		return -1
	}
	return 1
}

// testPackagesOf picks j's internal and external test packages from pkgs:
// the test variants whose files sit in j's directory.
func testPackagesOf(pkgs []*packages.Package, j *dirJob) []*packages.Package {
	var out []*packages.Package
	for _, p := range pkgs {
		if !strings.Contains(p.ID, " [") || len(p.GoFiles) == 0 {
			continue
		}
		if filepath.Dir(p.GoFiles[0]) == j.dir {
			out = append(out, p)
		}
	}
	return out
}

// writeOverlay writes overlay for the go command's -overlay flag, so it
// lists the migrated files' imports, not those on disk.
func writeOverlay(overlay map[string][]byte) (cleanup func(), flag string, err error) {
	if len(overlay) == 0 {
		return func() {}, "", nil
	}
	dir, err := os.MkdirTemp("", "gotest-migrate-overlay")
	if err != nil {
		return nil, "", err
	}
	cleanup = func() { _ = os.RemoveAll(dir) }
	replace := map[string]string{}
	i := 0
	for file, src := range overlay {
		i++
		path := filepath.Join(dir, strconv.Itoa(i)+"-"+filepath.Base(file))
		if err := os.WriteFile(path, src, 0o600); err != nil {
			cleanup()
			return nil, "", err
		}
		replace[file] = path
	}
	data, err := json.Marshal(struct{ Replace map[string]string }{replace})
	if err == nil {
		err = os.WriteFile(filepath.Join(dir, "overlay.json"), data, 0o600)
	}
	if err != nil {
		cleanup()
		return nil, "", err
	}
	return cleanup, "-overlay=" + filepath.Join(dir, "overlay.json"), nil
}

// newUnit builds the unit of j from its loaded packages: each test file's
// syntax from the first configuration that compiles it, type information
// merged across all of them. Files no configuration compiles are parsed
// only.
func newUnit(j *dirJob, ls []loaded, fset *token.FileSet, srcs map[string][]byte) (*unit, error) {
	u := &unit{
		fset:    fset,
		scopes:  map[string][]*types.Scope{},
		filePkg: map[*ast.File]*types.Package{},
	}
	var checked []*checkedPkg
	for _, l := range ls {
		for _, p := range l.pkgs {
			if p.info != nil && p.types != nil {
				checked = append(checked, p)
			}
		}
	}
	if len(checked) == 1 {
		u.info = checked[0].info // nothing to merge
	} else {
		u.info = &types.Info{
			Types:      map[ast.Expr]types.TypeAndValue{},
			Defs:       map[*ast.Ident]types.Object{},
			Uses:       map[*ast.Ident]types.Object{},
			Implicits:  map[ast.Node]types.Object{},
			Selections: map[*ast.SelectorExpr]*types.Selection{},
			Scopes:     map[ast.Node]*types.Scope{},
		}
		for _, p := range checked {
			mergeInfo(u.info, p.info)
		}
	}
	byPath := map[string]*unitFile{}
	for _, l := range ls {
		for _, p := range l.pkgs {
			if p.info == nil || p.types == nil {
				continue
			}
			u.scopes[p.types.Path()] = append(u.scopes[p.types.Path()], p.types.Scope())
			for _, f := range p.syntax {
				path := fset.File(f.FileStart).Name()
				u.filePkg[f] = p.types
				if !strings.HasSuffix(path, "_test.go") || byPath[path] != nil {
					continue
				}
				uf := &unitFile{path: path, src: srcs[path], f: f}
				byPath[path] = uf
			}
		}
	}
	for _, path := range j.tests {
		uf := byPath[path]
		if uf == nil {
			src, err := os.ReadFile(path)
			if err != nil {
				return nil, fmt.Errorf("read %s: %w", path, err)
			}
			f, err := parser.ParseFile(fset, path, src, parser.ParseComments|parser.SkipObjectResolution)
			if err != nil {
				return nil, fmt.Errorf("parse %s: %w", path, err)
			}
			uf = &unitFile{path: path, src: src, f: f, unchecked: j.unchecked[path]}
			if uf.unchecked == "" {
				uf.unchecked = "no build configuration migrate checked"
			}
		}
		if uf.src == nil {
			return nil, errors.New("migrate: no source for " + path)
		}
		uf.generated = ast.IsGenerated(uf.f)
		u.files = append(u.files, uf)
	}
	return u, nil
}

func mergeInfo(dst, src *types.Info) {
	for k, v := range src.Types {
		dst.Types[k] = v
	}
	for k, v := range src.Defs {
		dst.Defs[k] = v
	}
	for k, v := range src.Uses {
		dst.Uses[k] = v
	}
	for k, v := range src.Implicits {
		dst.Implicits[k] = v
	}
	for k, v := range src.Selections {
		dst.Selections[k] = v
	}
	for k, v := range src.Scopes {
		dst.Scopes[k] = v
	}
}
