package migrate

import (
	"bytes"
	"fmt"
	"go/ast"
	"go/format"
	"go/scanner"
	"go/token"
	"go/types"
	"sort"
	"strings"

	"golang.org/x/tools/imports"
)

// todoAnnotation marks a line in the migrated output that needs a
// TODO(gotest-migrate) comment inserted above it.
type todoAnnotation struct {
	line int
	msg  string
}

// lineMarks collects annotations, one per line.
type lineMarks struct {
	fset   *token.FileSet
	anns   []todoAnnotation
	marked map[int]bool
}

func (m *lineMarks) add(pos token.Pos, msg string) {
	m.addLine(m.fset.Position(pos).Line, msg)
}

func (m *lineMarks) addLine(line int, msg string) {
	if m.marked == nil {
		m.marked = map[int]bool{}
	}
	if m.marked[line] {
		return // one marker per line
	}
	m.marked[line] = true
	m.anns = append(m.anns, todoAnnotation{line: line, msg: msg})
}

// unconverted lists what in the migrated f still needs a hand: testify
// hooks gotest never calls, hooks and tests promoted from an embedded type,
// assertions left unrewritten, s.T() and s.Run without an equivalent in
// scope, anything else the dropped suite.Suite embedding provided, and a
// suite.Run left to run a migrated suite. u is the migrated unit, its
// suites keyed by their new names.
func (u *unit) unconverted(f *ast.File, m *lineMarks) {
	for _, decl := range f.Decls {
		if gd, ok := decl.(*ast.GenDecl); ok && gd.Tok == token.TYPE {
			u.unconvertedTypes(gd, m)
			continue
		}
		fd, ok := decl.(*ast.FuncDecl)
		if !ok {
			continue
		}
		var suite *suiteDecl
		if fd.Recv != nil && len(fd.Recv.List) > 0 {
			suite = u.suiteOf(fd.Recv.List[0].Type)
		}
		pos := fd.Pos()
		if fd.Doc != nil {
			pos = fd.Doc.Pos()
		}
		if suite != nil && suite.reserved[fd.Name.Name] != "" {
			m.add(pos, suite.reserved[fd.Name.Name])
		}
		if suite != nil && unsupportedHooks[fd.Name.Name] {
			m.add(pos, "unsupported testify hook "+fd.Name.Name+" — convert manually")
		}
		if suite != nil && gotestCalls(fd.Name.Name) {
			if _, ptr := fd.Recv.List[0].Type.(*ast.StarExpr); !ptr {
				m.add(pos, fd.Name.Name+" has a value receiver: gotest calls only pointer-receiver methods, and testify ran this one on a copy it then dropped, so the changes it makes to the suite were lost — make the receiver *"+suite.newName+" if they should last")
			}
		}
		if fd.Body == nil {
			continue
		}
		tName, kind, _ := tParam(fd, u.info)
		related := suite != nil || u.takesSuite(fd)
		calls := map[*ast.SelectorExpr]bool{}
		ast.Inspect(fd.Body, func(n ast.Node) bool {
			switch x := n.(type) {
			case *ast.CallExpr:
				if sel, ok := x.Fun.(*ast.SelectorExpr); ok {
					calls[sel] = true
				}
				if msg := u.unconvertedCall(f, x, tName, kind, related, fd.Name.Name); msg != "" {
					m.add(x.Pos(), msg)
				}
			case *ast.SelectorExpr:
				if v := u.suiteVar(x.X); v != nil {
					sel := u.info.Selections[x]
					switch {
					case sel == nil:
						// Whatever else came from the suite.Suite embedding.
						m.add(x.Pos(), v.Name+"."+x.Sel.Name+" came from the suite.Suite embedding, which is gone — convert manually")
					case !calls[x] && takesGotestT(sel.Obj()):
						m.add(x.Pos(), "method value "+v.Name+"."+x.Sel.Name+": "+x.Sel.Name+" now takes t *gotest.T, so it has a new type — call it with t instead")
					}
				}
			}
			return true
		})
	}
}

// gotestCalls reports whether gotest calls a suite method of this name.
func gotestCalls(name string) bool {
	switch name {
	case "BeforeAll", "AfterAll", "BeforeEach", "AfterEach":
		return true
	}
	return strings.HasPrefix(name, "Test")
}

// takesGotestT reports whether obj is a method taking *gotest.T first.
func takesGotestT(obj types.Object) bool {
	sig, ok := obj.Type().(*types.Signature)
	if !ok || sig.Params().Len() == 0 {
		return false
	}
	p, ok := sig.Params().At(0).Type().(*types.Pointer)
	if !ok {
		return false
	}
	n, ok := p.Elem().(*types.Named)
	return ok && n.Obj().Name() == "T" && n.Obj().Pkg() != nil && n.Obj().Pkg().Path() == gotestImport
}

// unconvertedTypes marks, on a migrated suite's type, the notes the
// migration left for it and the hooks or tests it only has through an
// embedded type: gotest calls the methods a suite declares itself.
func (u *unit) unconvertedTypes(gd *ast.GenDecl, m *lineMarks) {
	for _, spec := range gd.Specs {
		ts, ok := spec.(*ast.TypeSpec)
		if !ok {
			continue
		}
		s := u.suiteFor(u.info.Defs[ts.Name])
		if s == nil {
			continue
		}
		if s.note != "" {
			m.add(specPos(gd, ts), s.note)
		}
		st, ok := ts.Type.(*ast.StructType)
		obj, _ := u.info.Defs[ts.Name].(*types.TypeName)
		if !ok || obj == nil {
			continue
		}
		mset := types.NewMethodSet(types.NewPointer(obj.Type()))
		promoted := map[int][]string{} // by embedded field index
		for i := range mset.Len() {
			sel := mset.At(i)
			name := sel.Obj().Name()
			if len(sel.Index()) < 2 || !gotestCalls(name) && lifecycleRenames[name] == "" && !unsupportedHooks[name] {
				continue
			}
			promoted[sel.Index()[0]] = append(promoted[sel.Index()[0]], name)
		}
		index := 0
		for _, field := range st.Fields.List {
			n := max(len(field.Names), 1)
			if len(field.Names) == 0 && promoted[index] != nil {
				names := promoted[index]
				sort.Strings(names)
				m.add(field.Pos(), strings.Join(names, ", ")+" "+pick(len(names) == 1, "is", "are")+" promoted from "+types.ExprString(field.Type)+
					"; gotest calls only methods "+s.newName+" declares itself, so "+pick(len(names) == 1, "it no longer runs", "they no longer run")+" — declare them on "+s.newName+" (hooks under their gotest names, with t) and call the embedded ones")
			}
			index += n
		}
	}
}

// unconvertedCall is the marker message for a call the migration left as
// it was, empty for any other. tName and kind are the enclosing function's
// t; related says it is a suite method or takes a suite.
func (u *unit) unconvertedCall(f *ast.File, call *ast.CallExpr, tName, kind string, related bool, fn string) string {
	sel, ok := call.Fun.(*ast.SelectorExpr)
	if !ok {
		return ""
	}
	name := sel.Sel.Name
	gotestName, base, mapped := gotestAssertion(name)
	why := func(args []ast.Expr) string {
		_, _, why := u.mapAssertion(f, name, args)
		return why
	}
	spread := ""
	if mapped && base != name && call.Ellipsis.IsValid() {
		spread = name + " spreads args after its message, which gotest." + gotestName + " cannot take — rewrite as gotest." + gotestName + "(t, ..., append([]any{msg}, args...)...)"
	}

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
		case why(call.Args) != "":
			return why(call.Args)
		case spread != "":
			return spread
		case kind == "":
			return "assertion " + name + " outside a migrated method — no t is in scope here; pass t in and rewrite as gotest." + gotestName + "(t, ...)"
		}
		return "unconverted assertion " + name + " — rewrite as gotest." + gotestName + "(" + tName + ", ...)"
	case *ast.Ident:
		// assert.X(...) or require.X(...) in suite code: mapped names
		// were rewritten where they could be, and still compile otherwise.
		if u.isPkg(x, assertPkg, requirePkg) {
			switch {
			case related && !mapped && isTestifyAssertionName(name):
				return "unmapped assertion " + name + " — convert manually"
			case related && mapped && len(call.Args) > 0 && why(call.Args[1:]) != "":
				return why(call.Args[1:])
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
				return v.Name + ".T() where " + tName + " is a *testing.T — use " + tName
			case "gotest":
				return v.Name + ".T() — use " + tName + ".T()"
			}
			return v.Name + ".T() outside a migrated method — no t is in scope here"
		case name == "Run":
			if kind == "gotest" {
				return v.Name + ".Run has no gotest equivalent — use " + tName + ".It(name, func(" + tName + " *gotest.T) {...}); the assertions inside already name " + tName
			}
			return v.Name + ".Run has no gotest equivalent — use t.It(name, func(t *gotest.T) {...}) in a method with t"
		case mapped && why(call.Args) != "":
			return why(call.Args)
		case spread != "":
			return spread
		case isTestifyAssertionName(name) && mapped && kind == "":
			return "assertion " + name + " outside a migrated method — no t is in scope here; pass t in and rewrite as gotest." + gotestName + "(t, ...)"
		case isTestifyAssertionName(name) && mapped:
			return "unconverted assertion " + name + " — rewrite as gotest." + gotestName + "(" + tName + ", ...)"
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

// formatImports gofmts src and groups its imports the way goimports does:
// the standard library apart from everything else.
func formatImports(path string, src []byte) ([]byte, error) {
	return imports.Process(path, src, &imports.Options{FormatOnly: true, Comments: true, TabIndent: true, TabWidth: 8})
}

// insertMarkers writes one TODO(gotest-migrate) comment above each annotated
// line and returns the formatted result. A line that starts inside a raw
// string or a block comment takes its marker above the line that literal
// starts on, where it is a comment and leaves the literal as it was.
func insertMarkers(src []byte, anns []todoAnnotation) ([]byte, error) {
	if len(anns) == 0 {
		return src, nil
	}
	before, err := countSourceMarkers(src)
	if err != nil {
		return nil, err
	}

	starts := tokenStarts(src)
	for i := range anns {
		for starts[anns[i].line] > 0 {
			anns[i].line = starts[anns[i].line]
		}
	}
	// Insert comment lines bottom-up so earlier line numbers stay valid.
	sort.SliceStable(anns, func(i, j int) bool { return anns[i].line > anns[j].line })
	lines := strings.Split(string(src), "\n")
	inserted := 0
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
		inserted++
	}
	out, err := format.Source([]byte(strings.Join(lines, "\n")))
	if err != nil {
		return nil, err
	}
	// Every marker must be a comment, or the result would not count it.
	if after, err := countSourceMarkers(out); err != nil || after < before+inserted {
		return nil, fmt.Errorf("a marker did not land as a comment (%d of %d)", after-before, inserted)
	}
	return out, nil
}

// tokenStarts maps each line of src that starts inside a token, a raw
// string or a block comment spanning lines, to the line the token starts on.
func tokenStarts(src []byte) map[int]int {
	fset := token.NewFileSet()
	file := fset.AddFile("", -1, len(src))
	var sc scanner.Scanner
	sc.Init(file, src, nil, scanner.ScanComments)
	starts := map[int]int{}
	for {
		pos, tok, lit := sc.Scan()
		if tok == token.EOF {
			return starts
		}
		if tok != token.STRING && tok != token.COMMENT {
			continue
		}
		// Only raw strings and block comments span lines. The end is read
		// from src: the scanner drops their carriage returns from lit.
		off, end := file.Offset(pos), -1
		switch {
		case strings.HasPrefix(lit, "`"):
			end = bytes.IndexByte(src[off+1:], '`') + off + 1
		case strings.HasPrefix(lit, "/*"):
			end = bytes.Index(src[off:], []byte("*/")) + off + 1
		}
		if end <= off {
			continue
		}
		first, last := file.Line(pos), file.Line(file.Pos(end))
		for line := first + 1; line <= last; line++ {
			starts[line] = first
		}
	}
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
