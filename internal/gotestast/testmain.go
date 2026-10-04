package gotestast

import (
	"go/ast"
	"strconv"

	"github.com/mvrahden/go-test/internal/about"
)

// RuntimeMain names the call a user TestMain makes in place of m.Run in a
// package whose suites bind fixtures.
const RuntimeMain = "gotestruntime.Main(m)"

const runtimePkgPath = about.Repo + "/pkg/gotestruntime"

// FindTestMain returns the TestMain the developer wrote among files, and the
// file holding it. Generated files are skipped: a harness left on disk by
// gotest generate carries a TestMain of its own.
func FindTestMain(files []*ast.File) (*ast.FuncDecl, *ast.File) {
	for _, f := range files {
		if ast.IsGenerated(f) {
			continue
		}
		for _, decl := range f.Decls {
			if fd, ok := decl.(*ast.FuncDecl); ok && fd.Recv == nil && fd.Name.Name == "TestMain" {
				return fd, f
			}
		}
	}
	return nil, nil
}

// CallsRuntimeMain reports whether the TestMain fd, declared in file, calls
// gotestruntime.Main under whatever name file imports the runtime as.
func CallsRuntimeMain(file *ast.File, fd *ast.FuncDecl) bool {
	if fd.Body == nil {
		return false
	}
	names := map[string]bool{}
	for _, imp := range file.Imports {
		path, err := strconv.Unquote(imp.Path.Value)
		if err != nil || path != runtimePkgPath {
			continue
		}
		name := "gotestruntime"
		if imp.Name != nil {
			name = imp.Name.Name
		}
		names[name] = true
	}
	found := false
	ast.Inspect(fd.Body, func(n ast.Node) bool {
		call, ok := n.(*ast.CallExpr)
		if !ok || found {
			return !found
		}
		sel, ok := call.Fun.(*ast.SelectorExpr)
		if !ok || sel.Sel.Name != "Main" {
			return true
		}
		if id, ok := sel.X.(*ast.Ident); ok && names[id.Name] {
			found = true
		}
		return !found
	})
	return found
}
