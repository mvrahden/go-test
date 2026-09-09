package refactor

import (
	"fmt"
	"go/ast"
	"go/format"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"strings"

	"github.com/mvrahden/go-test/internal/gotestast"
)

// PromoteFuzzSeed locates the fuzz method identified by suiteName.methodName
// among dir's "_test.go" files, splices `<param>.Add(argExprs...)` into its
// body directly after the last existing top-level f.Add call (or as the
// first statement if none exist), gofmt-formats the result, and writes it
// back to disk. It returns the absolute path of the edited file and the
// 1-based line number where the new statement landed.
//
// If the method can't be located unambiguously, or splicing would produce
// invalid Go, no file is modified and an error is returned — this never
// partially edits or corrupts user source.
func PromoteFuzzSeed(dir, suiteName, methodName string, argExprs []string) (filePath string, line int, err error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return "", 0, fmt.Errorf("reading %s: %w", dir, err)
	}

	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), "_test.go") {
			continue
		}
		p := filepath.Join(dir, entry.Name())
		src, err := os.ReadFile(p)
		if err != nil {
			return "", 0, fmt.Errorf("reading %s: %w", p, err)
		}

		edited, found, err := InsertFuzzAdd(src, suiteName, methodName, argExprs)
		if err != nil {
			return "", 0, fmt.Errorf("%s: %w", p, err)
		}
		if !found {
			continue
		}

		if err := os.WriteFile(p, edited, 0600); err != nil {
			return "", 0, fmt.Errorf("writing %s: %w", p, err)
		}

		newLine, err := lastAddLine(edited, suiteName, methodName)
		if err != nil {
			// The write already succeeded; a line number is best-effort
			// display info only, so fall back to 0 rather than failing.
			newLine = 0
		}
		return p, newLine, nil
	}

	return "", 0, fmt.Errorf("fuzz method %s.%s not found in %s", suiteName, methodName, dir)
}

// InsertFuzzAdd parses src (a single Go source file's bytes) looking for the
// fuzz method suiteName.methodName. If found, it splices
// `<param>.Add(argExprs...)` into the method body directly after the last
// existing top-level `<param>.Add(...)` call (or as the first statement if
// none exist), where <param> is the method's *gotest.F parameter name, and
// returns the gofmt-formatted result with found=true.
//
// If the method is not present in src, it returns found=false and a nil
// error — callers scanning multiple candidate files use this to try the
// next one. err is non-nil only once the method HAS been located but the
// edit can't be produced or re-parsed as valid Go, so callers can tell "try
// another file" apart from "found it, but something is wrong" and abort
// instead of silently skipping a genuine problem.
func InsertFuzzAdd(src []byte, suiteName, methodName string, argExprs []string) (edited []byte, found bool, err error) {
	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, "", src, parser.ParseComments)
	if err != nil {
		return nil, false, fmt.Errorf("parsing source: %w", err)
	}

	decl := findFuzzFuncDecl(file, suiteName, methodName)
	if decl == nil {
		return nil, false, nil
	}
	if decl.Body == nil {
		return nil, true, fmt.Errorf("fuzz method %s.%s has no body", suiteName, methodName)
	}

	paramName, ok := fuzzFParamName(decl)
	if !ok {
		return nil, true, fmt.Errorf("fuzz method %s.%s: could not identify its *gotest.F parameter", suiteName, methodName)
	}

	insertOffset, ok := addInsertOffset(fset, decl, paramName)
	if !ok || insertOffset < 0 || insertOffset > len(src) {
		return nil, true, fmt.Errorf("fuzz method %s.%s: could not determine an insertion point", suiteName, methodName)
	}

	newStmt := fmt.Sprintf("\n\t%s.Add(%s)", paramName, strings.Join(argExprs, ", "))
	spliced := make([]byte, 0, len(src)+len(newStmt))
	spliced = append(spliced, src[:insertOffset]...)
	spliced = append(spliced, newStmt...)
	spliced = append(spliced, src[insertOffset:]...)

	formatted, err := format.Source(spliced)
	if err != nil {
		return nil, true, fmt.Errorf("formatting edited source: %w", err)
	}
	return formatted, true, nil
}

// findFuzzFuncDecl returns the FuncDecl in file whose receiver type is
// suiteName and whose name is methodName, or nil if none matches.
func findFuzzFuncDecl(file *ast.File, suiteName, methodName string) *ast.FuncDecl {
	for _, decl := range file.Decls {
		fd, ok := decl.(*ast.FuncDecl)
		if !ok || fd.Recv == nil || len(fd.Recv.List) == 0 {
			continue
		}
		if fd.Name.Name != methodName {
			continue
		}
		if gotestast.ReceiverTypeName(fd.Recv.List[0].Type) != suiteName {
			continue
		}
		return fd
	}
	return nil
}

// fuzzFParamName returns the name of decl's *gotest.F parameter.
func fuzzFParamName(decl *ast.FuncDecl) (string, bool) {
	if decl.Type.Params == nil {
		return "", false
	}
	for _, field := range decl.Type.Params.List {
		if !isGotestFStar(field.Type) {
			continue
		}
		if len(field.Names) != 1 {
			return "", false
		}
		return field.Names[0].Name, true
	}
	return "", false
}

// isGotestFStar reports whether expr is the syntactic shape `*gotest.F`.
func isGotestFStar(expr ast.Expr) bool {
	star, ok := expr.(*ast.StarExpr)
	if !ok {
		return false
	}
	sel, ok := star.X.(*ast.SelectorExpr)
	if !ok {
		return false
	}
	pkgIdent, ok := sel.X.(*ast.Ident)
	return ok && pkgIdent.Name == "gotest" && sel.Sel.Name == "F"
}

// addInsertOffset returns the byte offset in the original source at which a
// new `<param>.Add(...)` statement should be spliced: right after the last
// existing top-level `<param>.Add(...)` call in decl's body, or right after
// the opening brace if none exist.
func addInsertOffset(fset *token.FileSet, decl *ast.FuncDecl, paramName string) (int, bool) {
	lastIdx := -1
	for i, stmt := range decl.Body.List {
		if isParamAddCall(stmt, paramName) {
			lastIdx = i
		}
	}
	if lastIdx >= 0 {
		return fset.Position(decl.Body.List[lastIdx].End()).Offset, true
	}
	if !decl.Body.Lbrace.IsValid() {
		return -1, false
	}
	return fset.Position(decl.Body.Lbrace).Offset + 1, true
}

// isParamAddCall reports whether stmt is a top-level `<paramName>.Add(...)`
// expression statement.
func isParamAddCall(stmt ast.Stmt, paramName string) bool {
	es, ok := stmt.(*ast.ExprStmt)
	if !ok {
		return false
	}
	ce, ok := es.X.(*ast.CallExpr)
	if !ok {
		return false
	}
	sel, ok := ce.Fun.(*ast.SelectorExpr)
	if !ok || sel.Sel.Name != "Add" {
		return false
	}
	ident, ok := sel.X.(*ast.Ident)
	return ok && ident.Name == paramName
}

// lastAddLine re-parses edited (post-format) source and returns the 1-based
// line of the last `<param>.Add(...)` call in suiteName.methodName's body —
// i.e. the statement InsertFuzzAdd just spliced in.
func lastAddLine(edited []byte, suiteName, methodName string) (int, error) {
	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, "", edited, 0)
	if err != nil {
		return 0, err
	}
	decl := findFuzzFuncDecl(file, suiteName, methodName)
	if decl == nil || decl.Body == nil {
		return 0, fmt.Errorf("fuzz method %s.%s not found after edit", suiteName, methodName)
	}
	paramName, ok := fuzzFParamName(decl)
	if !ok {
		return 0, fmt.Errorf("fuzz method %s.%s: could not identify its *gotest.F parameter", suiteName, methodName)
	}
	lastIdx := -1
	for i, stmt := range decl.Body.List {
		if isParamAddCall(stmt, paramName) {
			lastIdx = i
		}
	}
	if lastIdx < 0 {
		return 0, fmt.Errorf("fuzz method %s.%s: no Add call found after edit", suiteName, methodName)
	}
	return fset.Position(decl.Body.List[lastIdx].Pos()).Line, nil
}
