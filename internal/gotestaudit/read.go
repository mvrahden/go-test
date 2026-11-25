package gotestaudit

import (
	"cmp"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"golang.org/x/tools/go/packages"

	"github.com/mvrahden/go-test/internal/about"
	"github.com/mvrahden/go-test/internal/gotestgen"
	"github.com/mvrahden/go-test/internal/gotestspec"
	"github.com/mvrahden/go-test/internal/lint"
)

// Options say which tree to read and how its packages load.
type Options struct {
	Patterns []string
	// LoadFlags go to the go command: -tags, say.
	LoadFlags  []string
	Provenance Provenance
}

// Read records the tree in the current directory. Packages that do not
// compile are recorded as unreadable; the error is for a load that failed
// as a whole. Unmatched are the patterns that matched no package.
func Read(ctx context.Context, opts *Options) (rec Record, unmatched []string, err error) {
	pkgs, err := gotestgen.LoadRunPackages(opts.Patterns, opts.LoadFlags)
	if err != nil {
		return Record{}, nil, err
	}
	rec = Record{
		Version:    about.ResolvedVersion(),
		Schema:     Schema,
		Provenance: opts.Provenance,
		Patterns:   slices.Clone(opts.Patterns),
		Notes:      []string{FloorNote},
	}
	var loaded []string
	for _, lr := range pkgs.Loaded {
		rec.Packages = append(rec.Packages, readPackage(lr))
		loaded = append(loaded, lr.PkgPath)
		rec.Snapshots = append(rec.Snapshots, snapshotsIn(lr.PkgDir, moduleOf(lr))...)
	}
	for i := range pkgs.Broken {
		pkg, m := brokenPackage(&pkgs.Broken[i])
		rec.Packages = append(rec.Packages, pkg)
		rec.Snapshots = append(rec.Snapshots, snapshotsIn(pkgs.Broken[i].Dir, m)...)
	}
	slices.SortFunc(rec.Packages, func(a, b Package) int { return cmp.Compare(a.Path, b.Path) })
	slices.SortFunc(rec.Snapshots, func(a, b Snapshot) int {
		return cmp.Or(cmp.Compare(a.Module, b.Module), cmp.Compare(a.File, b.File))
	})
	if len(loaded) > 0 {
		supp, err := suppressions(ctx, loaded, opts.LoadFlags)
		if err != nil {
			rec.Unread = append(rec.Unread, "suppressions: "+err.Error())
		}
		rec.Suppressions = supp
	}
	rec.normalize()
	return rec, pkgs.Unmatched, nil
}

// normalize gives every list a value, so a document says [] for none.
func (r *Record) normalize() {
	if r.Patterns == nil {
		r.Patterns = []string{}
	}
	if r.Packages == nil {
		r.Packages = []Package{}
	}
	if r.Suppressions == nil {
		r.Suppressions = []Suppression{}
	}
	if r.Snapshots == nil {
		r.Snapshots = []Snapshot{}
	}
	if r.Notes == nil {
		r.Notes = []string{}
	}
	for i := range r.Packages {
		if r.Packages[i].Suites == nil {
			r.Packages[i].Suites = []Suite{}
		}
		for j := range r.Packages[i].Suites {
			s := &r.Packages[i].Suites[j]
			if s.Methods == nil {
				s.Methods = []Behavior{}
			}
			normalizeBehaviors(s.Methods)
		}
	}
}

func normalizeBehaviors(bs []Behavior) {
	for i := range bs {
		if bs[i].Children == nil {
			bs[i].Children = []Behavior{}
		}
		normalizeBehaviors(bs[i].Children)
	}
}

// module is a package's module: its path and root directory.
type module struct{ path, dir string }

func moduleOf(lr *gotestgen.LoadResult) module {
	for _, p := range []*packages.Package{lr.Ptest, lr.Pxtest} {
		if p != nil && p.Module != nil {
			return module{p.Module.Path, p.Module.Dir}
		}
	}
	return module{dir: rootWithGoMod(lr.PkgDir)}
}

// rootWithGoMod is the nearest directory at or above dir holding a go.mod,
// dir itself when none does.
func rootWithGoMod(dir string) string {
	for root := dir; ; {
		if _, err := os.Stat(filepath.Join(root, "go.mod")); err == nil {
			return root
		}
		parent := filepath.Dir(root)
		if parent == root {
			return dir
		}
		root = parent
	}
}

// rel is path relative to the module root, slash-separated; path itself
// where the root does not hold it.
func (m module) rel(path string) string {
	if m.dir != "" {
		if r, err := filepath.Rel(m.dir, path); err == nil && r != ".." && !strings.HasPrefix(r, ".."+string(filepath.Separator)) {
			return filepath.ToSlash(r)
		}
	}
	return filepath.ToSlash(path)
}

// brokenPackage records a package that does not compile, its errors with
// the module root left out.
func brokenPackage(b *gotestgen.BrokenPackage) (Package, module) {
	root := rootWithGoMod(b.Dir)
	m := module{path: gotestspec.ReadModulePath(root), dir: root}
	errs := make([]string, 0, len(b.Errors))
	for _, e := range b.Errors {
		errs = append(errs, strings.ReplaceAll(e, root+string(filepath.Separator), ""))
	}
	if len(errs) == 0 {
		errs = append(errs, "the package does not compile")
	}
	return Package{Path: b.PkgPath, Module: m.path, Dir: m.rel(b.Dir), Unreadable: errs}, m
}

// snapshotsIn lists the baselines MatchSnapshot keeps beside a package's
// test files.
func snapshotsIn(pkgDir string, m module) []Snapshot {
	files, _ := filepath.Glob(filepath.Join(pkgDir, "testdata", "__snapshots__", "*.snap"))
	out := make([]Snapshot, 0, len(files))
	for _, f := range files {
		data, err := os.ReadFile(f) //nolint:gosec // G304: a baseline of the tree being recorded
		if err != nil {
			continue
		}
		sum := sha256.Sum256(data)
		out = append(out, Snapshot{Module: m.path, File: m.rel(f), SHA256: hex.EncodeToString(sum[:])})
	}
	return out
}

// suppressions are lint -suppressions' list of the packages.
func suppressions(ctx context.Context, pkgPaths, loadFlags []string) ([]Suppression, error) {
	res, err := lint.Analyze(ctx, &lint.Targets{Patterns: pkgPaths, Tests: true, BuildFlags: loadFlags})
	if err != nil {
		return nil, err
	}
	out := make([]Suppression, 0, len(res.Suppressions))
	for _, s := range res.Suppressions {
		out = append(out, Suppression{Module: s.Module, File: s.File, Line: s.Line, Rules: s.Rules, Reason: s.Reason})
	}
	return out, nil
}

// readPackage records lr's suites, or why they could not be read.
func readPackage(lr *gotestgen.LoadResult) Package {
	m := moduleOf(lr)
	pkg := Package{Path: lr.PkgPath, Module: m.path, Dir: m.rel(lr.PkgDir)}
	collector := gotestgen.NewCollector()
	for _, p := range []*packages.Package{lr.Ptest, lr.Pxtest} {
		if p == nil {
			continue
		}
		result := collector.CollectSuiteSpecs(p)
		if len(result.Errs) > 0 {
			for _, e := range result.Errs {
				msg := e.Err.Error()
				if pos := p.Fset.Position(e.Pos); pos.IsValid() {
					msg = fmt.Sprintf("%s:%d: %s", m.rel(pos.Filename), pos.Line, msg)
				}
				pkg.Unreadable = append(pkg.Unreadable, msg)
			}
			continue
		}
		for _, suite := range result.Suites {
			pkg.Suites = append(pkg.Suites, readSuite(suite, m, p == lr.Pxtest))
		}
	}
	if len(pkg.Unreadable) > 0 {
		pkg.Suites = nil
		return pkg
	}
	slices.SortFunc(pkg.Suites, func(a, b Suite) int {
		return cmp.Or(cmp.Compare(a.Name, b.Name), compareBool(a.External, b.External))
	})
	return pkg
}

func compareBool(a, b bool) int {
	switch {
	case a == b:
		return 0
	case !a:
		return -1
	default:
		return 1
	}
}
