package migrate

import (
	"go/ast"
	"go/format"
	"go/token"
	"go/types"
	"sort"
	"strings"
)

// todoAnnotation marks a line in the migrated output that needs a
// TODO(gotest-migrate) comment inserted above it.
type todoAnnotation struct {
	line int
	msg  string
}

// unconverted lists what in the migrated f still needs a hand: testify
// hooks gotest never calls, assertions left unrewritten, s.T() and s.Run
// without an equivalent in scope, anything else the dropped suite.Suite
// embedding provided, and a suite.Run left to run a migrated suite. u is
// the migrated unit, its byObj keyed by the suites' new types.
func (u *unit) unconverted(f *ast.File) []todoAnnotation {
	var anns []todoAnnotation
	marked := map[int]bool{}
	add := func(pos token.Pos, msg string) {
		line := u.fset.Position(pos).Line
		if marked[line] {
			return // one marker per line
		}
		marked[line] = true
		anns = append(anns, todoAnnotation{line: line, msg: msg})
	}

	for _, decl := range f.Decls {
		fd, ok := decl.(*ast.FuncDecl)
		if !ok {
			continue
		}
		onSuite := fd.Recv != nil && len(fd.Recv.List) > 0 && u.suiteOf(fd.Recv.List[0].Type) != nil
		if onSuite && unsupportedHooks[fd.Name.Name] {
			pos := fd.Pos()
			if fd.Doc != nil {
				pos = fd.Doc.Pos()
			}
			add(pos, "unsupported testify hook "+fd.Name.Name+" — convert manually")
		}
		if fd.Body == nil {
			continue
		}
		kind := tParamKind(fd)
		related := onSuite || u.takesSuite(fd)
		ast.Inspect(fd.Body, func(n ast.Node) bool {
			switch x := n.(type) {
			case *ast.CallExpr:
				if msg := u.unconvertedCall(x, kind, related, fd.Name.Name); msg != "" {
					add(x.Pos(), msg)
				}
			case *ast.SelectorExpr:
				// Whatever else came from the suite.Suite embedding.
				if v := u.suiteVar(x.X); v != nil && u.info.Selections[x] == nil {
					add(x.Pos(), v.Name+"."+x.Sel.Name+" came from the suite.Suite embedding, which is gone — convert manually")
				}
			}
			return true
		})
	}
	return anns
}

// unconvertedCall is the marker message for a call the migration left as
// it was, empty for any other. kind is the enclosing function's t; related
// says it is a suite method or takes a suite.
func (u *unit) unconvertedCall(call *ast.CallExpr, kind string, related bool, fn string) string {
	sel, ok := call.Fun.(*ast.SelectorExpr)
	if !ok {
		return ""
	}
	name := sel.Sel.Name
	gotestName, mapped := assertionMap[name]

	switch x := sel.X.(type) {
	case *ast.CallExpr:
		// s.Require().X(...) or s.Assert().X(...)
		inner, ok := x.Fun.(*ast.SelectorExpr)
		if !ok || u.suiteVar(inner.X) == nil || (inner.Sel.Name != "Require" && inner.Sel.Name != "Assert") {
			return ""
		}
		switch {
		case !mapped:
			return "unmapped assertion " + name + " — convert manually"
		case kind == "":
			return "assertion " + name + " outside a migrated method — no t is in scope here; pass t in and rewrite as gotest." + gotestName + "(t, ...)"
		}
		return "unconverted assertion " + name + " — rewrite as gotest." + gotestName + "(t, ...)"
	case *ast.Ident:
		// assert.X(...) or require.X(...) in suite code: mapped names
		// were rewritten where they could be, and still compile otherwise.
		if u.isPkg(x, assertPkg, requirePkg) {
			if related && !mapped && isTestifyAssertionName(name) {
				return "unmapped assertion " + name + " — convert manually"
			}
			return ""
		}
		if u.isPkg(x, suitePkg) && name == "Run" && len(call.Args) == 2 && u.suiteOfType(u.info.TypeOf(call.Args[1])) != nil {
			return "suite.Run cannot run a gotest suite — gotest generates the runner; move what " + fn + " sets up into the suite (BeforeAll, SuiteGuard) and delete it"
		}
		v := u.suiteVar(x)
		if v == nil {
			return ""
		}
		switch {
		case name == "T" && len(call.Args) == 0:
			switch kind {
			case "testing":
				return v.Name + ".T() where t is a *testing.T — use t"
			case "gotest":
				return v.Name + ".T() — use t.T()"
			}
			return v.Name + ".T() outside a migrated method — no t is in scope here"
		case name == "Run":
			return v.Name + ".Run has no gotest equivalent — use t.When or t.It"
		case isTestifyAssertionName(name) && mapped:
			return "unconverted assertion " + name + " — embedded-suite call; rewrite as gotest." + gotestName + "(t, ...)"
		case isTestifyAssertionName(name):
			return "unmapped assertion " + name + " — convert manually"
		}
	}
	return ""
}

// suiteVar is the identifier expr is when it names a variable of a
// migrated suite type, nil otherwise.
func (u *unit) suiteVar(expr ast.Expr) *ast.Ident {
	ident, ok := expr.(*ast.Ident)
	if !ok {
		return nil
	}
	v, ok := u.info.Uses[ident].(*types.Var)
	if !ok || u.suiteOfType(v.Type()) == nil {
		return nil
	}
	return ident
}

// takesSuite reports whether fd has a parameter of a migrated suite type.
func (u *unit) takesSuite(fd *ast.FuncDecl) bool {
	if fd.Type.Params == nil {
		return false
	}
	for _, p := range fd.Type.Params.List {
		if u.suiteOf(p.Type) != nil {
			return true
		}
	}
	return false
}

// insertMarkers writes one TODO(gotest-migrate) comment above each annotated
// line and returns the formatted result.
func insertMarkers(src []byte, anns []todoAnnotation) ([]byte, error) {
	if len(anns) == 0 {
		return src, nil
	}

	// Insert comment lines bottom-up so earlier line numbers stay valid.
	sort.SliceStable(anns, func(i, j int) bool { return anns[i].line > anns[j].line })
	lines := strings.Split(string(src), "\n")
	for _, a := range anns {
		idx := a.line - 1
		if idx < 0 || idx >= len(lines) {
			continue
		}
		comment := markerPrefix + " " + a.msg
		if idx > 0 && strings.Contains(lines[idx-1], comment) {
			continue // already annotated (idempotency)
		}
		indented := leadingWhitespace(lines[idx]) + comment
		lines = append(lines[:idx], append([]string{indented}, lines[idx:]...)...)
	}
	return format.Source([]byte(strings.Join(lines, "\n")))
}

// leadingWhitespace returns the leading spaces/tabs of a line.
func leadingWhitespace(s string) string {
	for i, r := range s {
		if r != ' ' && r != '\t' {
			return s[:i]
		}
	}
	return s
}
