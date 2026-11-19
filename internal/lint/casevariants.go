package lint

import (
	"go/ast"
	"go/token"
	"go/types"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"golang.org/x/tools/go/analysis"
	"golang.org/x/tools/go/packages"

	"github.com/mvrahden/go-test/internal/about"
	"github.com/mvrahden/go-test/internal/gotestast"
)

// variant is what dropped-case reads of one test package.
type variant struct {
	info  *types.Info
	files []*ast.File
}

// caseUses is what decides whether a case of a directory runs: the methods
// its test packages refer to, the interface methods they name, the types an
// alias of theirs names as a suite, and, where both test packages were
// loaded, the types the generator refuses.
type caseUses struct {
	called  map[*types.Func]bool
	byName  map[string]bool
	aliased map[*types.TypeName]bool
	// orphans is nil when only one test package was seen.
	orphans map[*types.TypeName]bool
}

func newCaseUses(variants ...variant) *caseUses {
	c := &caseUses{called: map[*types.Func]bool{}, byName: map[string]bool{}, aliased: map[*types.TypeName]bool{}}
	for _, v := range variants {
		for _, obj := range v.info.Uses {
			fn, ok := obj.(*types.Func)
			if !ok {
				continue
			}
			if recv := fn.Signature().Recv(); recv != nil && types.IsInterface(recv.Type()) {
				c.byName[fn.Name()] = true
				continue
			}
			c.called[fn.Origin()] = true
		}
		for _, file := range v.files {
			for _, decl := range file.Decls {
				gd, ok := decl.(*ast.GenDecl)
				if !ok || gd.Tok != token.TYPE {
					continue
				}
				for _, spec := range gd.Specs {
					ts, ok := spec.(*ast.TypeSpec)
					if !ok || !ts.Assign.IsValid() || !gotestast.IS_TEST_SUITE.MatchString(ts.Name.Name) {
						continue
					}
					if named, ok := types.Unalias(v.info.TypeOf(ts.Type)).(*types.Named); ok {
						c.aliased[named.Origin().Obj()] = true
					}
				}
			}
		}
	}
	return c
}

// dirCases holds, while gotest lint analyzes, the uses of each loaded test
// package's directory, by package.
var dirCases sync.Map // *types.Package → *caseUses

func casesOfDir(pkg *types.Package) *caseUses {
	v, _ := dirCases.Load(pkg)
	c, _ := v.(*caseUses)
	return c
}

// registerDirCases records the uses of the test packages of every directory
// pkgs load, with the generator's verdict on the cases there, and returns
// the call that forgets them.
func registerDirCases(pkgs []*packages.Package) (forget func()) {
	type pair struct{ internal, external *packages.Package }
	byDir := map[string]*pair{}
	for _, p := range pkgs {
		forTest := testedBy(p.ID)
		if p.Types == nil || p.TypesInfo == nil || forTest == "" {
			continue
		}
		pr := byDir[forTest]
		if pr == nil {
			pr = &pair{}
			byDir[forTest] = pr
		}
		if p.PkgPath == forTest {
			pr.internal = p
		} else {
			pr.external = p
		}
	}
	var keys []*types.Package
	for _, pr := range byDir {
		var vs []variant
		for _, p := range []*packages.Package{pr.internal, pr.external} {
			if p != nil {
				vs = append(vs, variant{p.TypesInfo, p.Syntax})
			}
		}
		c := newCaseUses(vs...)
		c.orphans = map[*types.TypeName]bool{}
		for _, o := range gotestast.Orphans(pr.internal, pr.external) {
			if obj := receiverAt(o.Pos, pr.internal, pr.external); obj != nil {
				c.orphans[obj] = true
			}
		}
		for _, p := range []*packages.Package{pr.internal, pr.external} {
			if p != nil {
				dirCases.Store(p.Types, c)
				keys = append(keys, p.Types)
			}
		}
	}
	return func() {
		for _, k := range keys {
			dirCases.Delete(k)
		}
	}
}

// testedBy is the package a test variant's ID ("p [p.test]", "p_test
// [p.test]") is built for, "" for any other package.
func testedBy(id string) string {
	_, rest, ok := strings.Cut(id, " [")
	if !ok {
		return ""
	}
	forTest, ok := strings.CutSuffix(rest, ".test]")
	if !ok {
		return ""
	}
	return forTest
}

// receiverAt is the receiver type of the method whose name is at pos.
func receiverAt(pos token.Pos, pkgs ...*packages.Package) *types.TypeName {
	for _, p := range pkgs {
		if p == nil {
			continue
		}
		for id, obj := range p.TypesInfo.Defs {
			if id.Pos() != pos {
				continue
			}
			if fn, ok := obj.(*types.Func); ok && fn.Signature().Recv() != nil {
				if named := namedReceiver(fn); named != nil {
					return named.Obj()
				}
			}
		}
	}
	return nil
}

// onlyTestPackage reports whether every test file of the pass's directory is
// in the pass: then the package it sees is all a case can be reached from.
func onlyTestPackage(pass *analysis.Pass) bool {
	own := map[string]bool{}
	var dir string
	for _, f := range pass.Files {
		name := filepath.Clean(pass.Fset.File(f.Pos()).Name())
		own[name] = true
		if strings.HasSuffix(name, "_test.go") {
			dir = filepath.Dir(name)
		}
	}
	if dir == "" {
		return true // no test file: nothing to report
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		return false
	}
	for _, e := range entries {
		name := e.Name()
		if e.IsDir() || !strings.HasSuffix(name, "_test.go") || about.PSuiteRegex.MatchString(name) {
			continue
		}
		if !own[filepath.Join(dir, name)] {
			return false
		}
	}
	return true
}
