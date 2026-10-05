package gotestast

import (
	"go/ast"
	"go/token"
	"strconv"
	"strings"

	"github.com/mvrahden/go-test/internal/about"
)

// RuntimeMain names the call a user TestMain makes in place of m.Run in a
// package whose suites bind fixtures; RuntimeM the wrapper it passes to a
// library that runs the tests itself.
const (
	RuntimeMain = "gotestruntime.Main(m)"
	RuntimeM    = "gotestruntime.M(m)"
)

const runtimePkgPath = about.Repo + "/pkg/gotestruntime"

// FindTestMain returns the TestMain the developer wrote among files, and the
// file holding it: a func(*testing.M) named TestMain in a _test.go file, which
// is what go test runs. A harness gotest generated is skipped, since a stale
// one on disk carries a TestMain of its own; a file another generator wrote is
// the developer's as far as go test is concerned.
func FindTestMain(fset *token.FileSet, files []*ast.File) (*ast.FuncDecl, *ast.File) {
	for _, f := range files {
		if !strings.HasSuffix(fset.Position(f.Package).Filename, "_test.go") || IsGotestHarness(f) {
			continue
		}
		for _, decl := range f.Decls {
			if fd, ok := decl.(*ast.FuncDecl); ok && fd.Recv == nil && fd.Name.Name == "TestMain" && takesTestingM(fd) {
				return fd, f
			}
		}
	}
	return nil, nil
}

// IsGotestHarness reports whether f carries the header of a harness gotest
// generated.
func IsGotestHarness(f *ast.File) bool {
	for _, cg := range f.Comments {
		if cg.Pos() >= f.Package {
			break
		}
		for _, c := range cg.List {
			if GEN_TESTSUITE_FILE.MatchString(c.Text) {
				return true
			}
		}
	}
	return false
}

// takesTestingM reports whether fd has the single *testing.M parameter go test
// requires of a TestMain, under whatever name the file imports testing as.
func takesTestingM(fd *ast.FuncDecl) bool {
	params := fd.Type.Params.List
	if len(params) != 1 || len(params[0].Names) > 1 {
		return false
	}
	star, ok := params[0].Type.(*ast.StarExpr)
	if !ok {
		return false
	}
	sel, ok := star.X.(*ast.SelectorExpr)
	return ok && sel.Sel.Name == "M"
}

// RoutesThroughRuntime reports whether the TestMain fd, declared in file,
// calls gotestruntime.Main or gotestruntime.M, under whatever name file
// imports the runtime as. It reads fd's body only: a call reached through a
// helper is not seen, which is why the runtime, not this, is the guarantee.
func RoutesThroughRuntime(file *ast.File, fd *ast.FuncDecl) bool {
	if fd.Body == nil {
		return false
	}
	names := map[string]bool{}
	dot := false
	for _, imp := range file.Imports {
		path, err := strconv.Unquote(imp.Path.Value)
		if err != nil || path != runtimePkgPath {
			continue
		}
		switch {
		case imp.Name == nil:
			names["gotestruntime"] = true
		case imp.Name.Name == ".":
			dot = true
		case imp.Name.Name != "_":
			names[imp.Name.Name] = true
		}
	}
	found := false
	ast.Inspect(fd.Body, func(n ast.Node) bool {
		call, ok := n.(*ast.CallExpr)
		if !ok || found {
			return !found
		}
		switch fn := call.Fun.(type) {
		case *ast.SelectorExpr:
			if id, ok := fn.X.(*ast.Ident); ok && names[id.Name] && (fn.Sel.Name == "Main" || fn.Sel.Name == "M") {
				found = true
			}
		case *ast.Ident:
			if dot && (fn.Name == "Main" || fn.Name == "M") {
				found = true
			}
		}
		return !found
	})
	return found
}
