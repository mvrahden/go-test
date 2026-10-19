package refactor

import (
	"errors"
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	gotypes "go/types"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"golang.org/x/tools/imports"

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
func PromoteFuzzSeed(dir, suiteName, methodName string, argExprs []string, types TypeNames) (filePath string, line int, err error) {
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

		edited, found, err := insertFuzzAdd(p, src, suiteName, methodName, argExprs, types)
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
// existing top-level `<param>.Add(...)` call and its trailing comment (or as
// the first statement if none exist), where <param> is the method's
// *gotest.F parameter name, and returns the result with found=true, formatted
// and with the imports the spliced expressions need. Corpus-file float
// specials (float64(NaN), float64(+Inf), ...) are spelled as Go on the way.
//
// If the method is not present in src, it returns found=false and a nil
// error — callers scanning multiple candidate files use this to try the
// next one. err is non-nil only once the method HAS been located but the
// edit can't be produced or re-parsed as valid Go, or the seed's value count
// differs from the f.Fuzz callback's arguments, so callers can tell "try
// another file" apart from "found it, but something is wrong" and abort
// instead of silently skipping a genuine problem.
func InsertFuzzAdd(src []byte, suiteName, methodName string, argExprs []string, types TypeNames) (edited []byte, found bool, err error) {
	return insertFuzzAdd("", src, suiteName, methodName, argExprs, types)
}

// TypeNames reports whether qual.name (name alone when qual is empty) names
// a type where the seed is spliced. Only a conversion to a type is a
// literal; a nil TypeNames admits the predeclared types alone.
type TypeNames func(qual, name string) bool

// insertFuzzAdd is InsertFuzzAdd with the file's path, which decides where
// imports are resolved from.
func insertFuzzAdd(filename string, src []byte, suiteName, methodName string, argExprs []string, types TypeNames) (edited []byte, found bool, err error) {
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

	// A seed with the wrong value count is refused before anything is
	// written: f.Add would reject it at run time, after the source changed.
	if arity, ok := declaredArity(decl, paramName); ok && arity != len(argExprs) {
		return nil, true, fmt.Errorf("fuzz method %s.%s takes %d argument%s, the seed has %d value%s",
			suiteName, methodName, arity, plural(arity), len(argExprs), plural(len(argExprs)))
	}

	// Only literal values become a seed: a call or an operator is code.
	for _, e := range argExprs {
		if err := checkSeedLiteral(e, types); err != nil {
			return nil, true, fmt.Errorf("seed value %s is not a literal: %w", e, err)
		}
	}

	insertOffset, ok := addInsertOffset(fset, file, decl, paramName)
	if !ok || insertOffset < 0 || insertOffset > len(src) {
		return nil, true, fmt.Errorf("fuzz method %s.%s: could not determine an insertion point", suiteName, methodName)
	}

	exprs := make([]string, len(argExprs))
	for i, e := range argExprs {
		exprs[i] = rewriteFloatSpecials(e)
	}
	newStmt := fmt.Sprintf("\n\t%s.Add(%s)", paramName, strings.Join(exprs, ", "))
	spliced := make([]byte, 0, len(src)+len(newStmt))
	spliced = append(spliced, src[:insertOffset]...)
	spliced = append(spliced, newStmt...)
	spliced = append(spliced, src[insertOffset:]...)

	formatted, err := imports.Process(filename, spliced, &imports.Options{Comments: true, TabIndent: true, TabWidth: 8})
	if err != nil {
		return nil, true, fmt.Errorf("formatting edited source: %w", err)
	}
	return formatted, true, nil
}

func plural(n int) string {
	if n == 1 {
		return ""
	}
	return "s"
}

// declaredArity counts the arguments the method's f.Fuzz callback takes
// after its *gotest.T; false when the callback is not a literal.
func declaredArity(decl *ast.FuncDecl, paramName string) (int, bool) {
	for _, stmt := range decl.Body.List {
		es, ok := stmt.(*ast.ExprStmt)
		if !ok {
			continue
		}
		ce, ok := es.X.(*ast.CallExpr)
		if !ok || len(ce.Args) != 1 {
			continue
		}
		sel, ok := ce.Fun.(*ast.SelectorExpr)
		if !ok || sel.Sel.Name != "Fuzz" {
			continue
		}
		if ident, ok := sel.X.(*ast.Ident); !ok || ident.Name != paramName {
			continue
		}
		lit, ok := ce.Args[0].(*ast.FuncLit)
		if !ok || lit.Type.Params == nil {
			return 0, false
		}
		n := 0
		for _, field := range lit.Type.Params.List {
			n += max(len(field.Names), 1)
		}
		return n - 1, n > 0
	}
	return 0, false
}

// floatSpecial matches the corpus file's spelling of a float special, which
// Go's encoder writes as %v and no Go source can name.
var floatSpecial = regexp.MustCompile(`^(float32|float64)\(([+-]?Inf|NaN)\)$`)

// rewriteFloatSpecials spells float64(NaN) and its kin as math calls.
func rewriteFloatSpecials(expr string) string {
	m := floatSpecial.FindStringSubmatch(strings.TrimSpace(expr))
	if m == nil {
		return expr
	}
	var call string
	switch m[2] {
	case "NaN":
		call = "math.NaN()"
	case "-Inf":
		call = "math.Inf(-1)"
	default:
		call = "math.Inf(1)"
	}
	if m[1] == "float32" {
		return "float32(" + call + ")"
	}
	return call
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
// existing top-level `<param>.Add(...)` call in decl's body and any comment
// trailing it on the same line, or right after the opening brace if none
// exist.
func addInsertOffset(fset *token.FileSet, file *ast.File, decl *ast.FuncDecl, paramName string) (int, bool) {
	lastIdx := -1
	for i, stmt := range decl.Body.List {
		if isParamAddCall(stmt, paramName) {
			lastIdx = i
		}
	}
	if lastIdx >= 0 {
		end := decl.Body.List[lastIdx].End()
		line := fset.Position(end).Line
		for _, cg := range file.Comments {
			if cg.Pos() > end && fset.Position(cg.Pos()).Line == line {
				end = cg.End()
			}
		}
		return fset.Position(end).Offset, true
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

// conversionMath are the math functions a seed literal may call: the ones
// that spell float specials and float bits.
var conversionMath = map[string]int{"NaN": 0, "Inf": 1, "Float64frombits": 1, "Float32frombits": 1}

// checkSeedLiteral admits a value literal as a corpus file or a codec's echo
// spells it: basic literals, signed ones, true/false/nil, conversions of a
// literal, composite literals of literals, &T{...} and &[]T{v}[0], and the
// math calls that spell float specials. Anything else is code.
func checkSeedLiteral(src string, types TypeNames) error {
	expr, err := parser.ParseExpr(src)
	if err != nil {
		return err
	}
	if types == nil {
		types = predeclaredType
	}
	return seedChecker{types}.literal(expr)
}

// predeclaredType admits the predeclared types alone.
func predeclaredType(qual, name string) bool {
	if qual != "" {
		return false
	}
	_, ok := gotypes.Universe.Lookup(name).(*gotypes.TypeName)
	return ok
}

type seedChecker struct{ types TypeNames }

func (c seedChecker) literal(e ast.Expr) error {
	switch x := e.(type) {
	case *ast.BasicLit:
		return nil
	case *ast.Ident:
		switch x.Name {
		case "true", "false", "nil", "NaN", "Inf":
			return nil
		}
		return fmt.Errorf("identifier %s", x.Name)
	case *ast.ParenExpr:
		return c.literal(x.X)
	case *ast.UnaryExpr:
		switch x.Op {
		case token.SUB, token.ADD:
			return c.literal(x.X)
		case token.AND:
			switch inner := x.X.(type) {
			case *ast.CompositeLit:
				return c.literal(inner)
			case *ast.IndexExpr:
				return c.literal(inner)
			}
		}
		return fmt.Errorf("operator %s", x.Op)
	case *ast.IndexExpr:
		// &[]T{v}[0]: the one addressable spelling of a pointer to a value.
		lit, ok := x.X.(*ast.CompositeLit)
		idx, isLit := x.Index.(*ast.BasicLit)
		if !ok || !isLit || idx.Kind != token.INT {
			return errors.New("index expression")
		}
		return c.literal(lit)
	case *ast.CompositeLit:
		if x.Type != nil {
			if err := c.typ(x.Type); err != nil {
				return err
			}
		}
		for _, elt := range x.Elts {
			if kv, ok := elt.(*ast.KeyValueExpr); ok {
				if _, ok := kv.Key.(*ast.Ident); !ok {
					if err := c.literal(kv.Key); err != nil {
						return err
					}
				}
				if err := c.literal(kv.Value); err != nil {
					return err
				}
				continue
			}
			if err := c.literal(elt); err != nil {
				return err
			}
		}
		return nil
	case *ast.CallExpr:
		if x.Ellipsis.IsValid() {
			return errors.New("spread call")
		}
		if sel, ok := x.Fun.(*ast.SelectorExpr); ok {
			if pkg, ok := sel.X.(*ast.Ident); ok && pkg.Name == "math" {
				n, known := conversionMath[sel.Sel.Name]
				if !known || len(x.Args) != n {
					return fmt.Errorf("call of math.%s", sel.Sel.Name)
				}
				for _, a := range x.Args {
					if err := c.literal(a); err != nil {
						return err
					}
				}
				return nil
			}
		}
		// A conversion: one literal argument to a type.
		if len(x.Args) != 1 {
			return errors.New("call")
		}
		if err := c.conversion(x.Fun); err != nil {
			return err
		}
		return c.literal(x.Args[0])
	}
	return fmt.Errorf("%T", e)
}

// conversion admits the callee of a conversion: a name only when it names a
// type, which tells a conversion from a function call.
func (c seedChecker) conversion(fun ast.Expr) error {
	switch x := fun.(type) {
	case *ast.Ident:
		if !c.types("", x.Name) {
			return fmt.Errorf("call of %s", x.Name)
		}
		return nil
	case *ast.SelectorExpr:
		if q, ok := x.X.(*ast.Ident); ok && c.types(q.Name, x.Sel.Name) {
			return nil
		}
		return fmt.Errorf("call of %s", gotypes.ExprString(fun))
	case *ast.ParenExpr:
		return c.conversion(x.X)
	}
	return c.typ(fun)
}

// typ admits the type expressions a seed literal names.
func (c seedChecker) typ(e ast.Expr) error {
	switch x := e.(type) {
	case *ast.Ident:
		return nil
	case *ast.SelectorExpr:
		if _, ok := x.X.(*ast.Ident); ok {
			return nil
		}
	case *ast.ParenExpr:
		return c.typ(x.X)
	case *ast.StarExpr:
		return c.typ(x.X)
	case *ast.ArrayType:
		if x.Len != nil {
			if err := c.literal(x.Len); err != nil {
				return err
			}
		}
		return c.typ(x.Elt)
	case *ast.MapType:
		if err := c.typ(x.Key); err != nil {
			return err
		}
		return c.typ(x.Value)
	case *ast.IndexExpr:
		if err := c.typ(x.X); err != nil {
			return err
		}
		return c.typ(x.Index)
	case *ast.IndexListExpr:
		if err := c.typ(x.X); err != nil {
			return err
		}
		for _, i := range x.Indices {
			if err := c.typ(i); err != nil {
				return err
			}
		}
		return nil
	}
	return fmt.Errorf("type %T", e)
}
