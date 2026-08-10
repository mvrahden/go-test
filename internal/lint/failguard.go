package lint

import (
	"fmt"
	"go/ast"
	"go/token"
	"go/types"
	"strings"

	"golang.org/x/tools/go/analysis"
	"golang.org/x/tools/go/ast/inspector"
)

// checkFailGuard flags if-statements whose sole purpose is to call
// gotest.Fail when a condition holds — an assertion expresses the same check
// directly, with a richer failure report. Fail halts the test exactly like a
// failing assertion (both funnel through Errorf+FailNow), so the rewrite
// preserves control flow: ||-chained conditions and else-if chains decompose
// into sequential assertions, where halting on the first failure reproduces
// the original short-circuit evaluation.
func checkFailGuard(pass *analysis.Pass, insp *inspector.Inspector) {
	// else-branch if-statements belong to their parent's chain and must never
	// be reported standalone: replacing one would leave `else <expr>` behind.
	// Preorder visits parents first, so marking suffices.
	elseChild := map[*ast.IfStmt]bool{}

	sourceCache := map[string][]byte{}
	readSource := func(name string) []byte {
		if content, ok := sourceCache[name]; ok {
			return content
		}
		content, err := pass.ReadFile(name)
		if err != nil {
			content = nil
		}
		sourceCache[name] = content
		return content
	}

	insp.Preorder([]ast.Node{(*ast.IfStmt)(nil)}, func(n ast.Node) {
		ifStmt := n.(*ast.IfStmt)
		if inner, ok := ifStmt.Else.(*ast.IfStmt); ok {
			elseChild[inner] = true
		}
		if elseChild[ifStmt] {
			return
		}

		units, hasInit := collectFailChain(pass, ifStmt)
		if len(units) == 0 {
			return
		}

		var plans []failGuardPlan
		fixable := !hasInit
		weakLabel := ""
		for _, u := range units {
			if u.forceReport {
				fixable = false
			}
			if !u.halting && !u.returns && weakLabel == "" {
				weakLabel = u.label
			}
			for _, cond := range splitOr(u.cond) {
				m, ok := mapBoolExpr(pass, cond, true)
				if !ok {
					m = conditionMapping{"False", []ast.Expr{cond}, "failure guard"}
				}
				plans = append(plans, failGuardPlan{
					m:       m,
					tArg:    u.tArg,
					msgArgs: u.msgArgs,
					qual:    u.qual,
				})
			}
		}

		var targets []string
		for _, p := range plans {
			targets = append(targets, p.m.target)
		}
		targetList := strings.Join(targets, " + ")

		desc := plans[0].m.desc
		switch {
		case len(units) > 1:
			desc = "else-if failure guard"
		case len(plans) > 1:
			desc = "or-chained failure guard"
		}

		// The if body evaluated message args only on failure; an assertion
		// evaluates them on every run. Calls could have side effects, and
		// index/selector/deref expressions can panic exactly when the guard
		// would not have fired (errs[0] guarded by len > 0) — so only
		// trivially total args keep the autofix.
		for _, p := range plans {
			if !msgArgsSafe(p.msgArgs) {
				fixable = false
			}
		}
		from := "if+" + units[0].label
		// A non-halting body makes the rewrite a semantic strengthening, not
		// an equivalence — say so, and never offer a fix.
		if weakLabel != "" {
			report(pass, FailGuard, ifStmt.Pos(), "use %s instead of %s for %s — assertions halt where %s continues", targetList, from, desc, weakLabel)
			return
		}
		trailing, interior := spanComments(pass, ifStmt)
		if !fixable || interior {
			report(pass, FailGuard, ifStmt.Pos(), "use %s instead of %s for %s", targetList, from, desc)
			return
		}

		indent := "\n" + sourceIndent(pass.Fset, readSource, ifStmt.Pos())
		var lines []string
		for _, p := range plans {
			lines = append(lines, renderAssertion(pass.Fset, p.qual, p.m.target, append([]ast.Expr{p.tArg}, p.m.args...), p.msgArgs))
		}
		lines[0] += trailing
		reportWithFix(pass, FailGuard, ifStmt.Pos(),
			[]analysis.SuggestedFix{{
				Message: fmt.Sprintf("use %s", targetList),
				TextEdits: []analysis.TextEdit{{
					Pos:     ifStmt.Pos(),
					End:     ifStmt.End(),
					NewText: []byte(strings.Join(lines, indent)),
				}},
			}},
			"use %s instead of %s for %s", targetList, from, desc)
	})
}

// failGuardUnit is one branch of an if / else-if chain whose body halts the
// test: the guard condition, the halting call's t expression, the message
// args to carry over, the qualifier to render the assertion with, a label
// naming the halting call for diagnostics, and whether a fix can be offered.
type failGuardUnit struct {
	cond        ast.Expr
	tArg        ast.Expr
	msgArgs     []ast.Expr
	qual        string
	label       string
	halting     bool
	returns     bool
	forceReport bool
}

type failGuardPlan struct {
	m       conditionMapping
	tArg    ast.Expr
	msgArgs []ast.Expr
	qual    string
}

// collectFailChain walks an if / else-if chain and returns one unit per
// branch. It returns nil unless every branch consists of exactly one
// test-halting call (plus at most an unreachable bare return) and no branch
// carries an init statement.
func collectFailChain(pass *analysis.Pass, ifStmt *ast.IfStmt) (units []failGuardUnit, hasInit bool) {
	for cur := ifStmt; ; {
		if cur.Init != nil {
			// The init variable is scoped to the if — hoisting it for an
			// assertion can break compilation, so such chains report only.
			hasInit = true
		}
		unit := extractHaltingUnit(pass, cur.Body)
		if unit == nil {
			return nil, false
		}
		unit.cond = cur.Cond
		units = append(units, *unit)
		switch e := cur.Else.(type) {
		case nil:
			return units, hasInit
		case *ast.IfStmt:
			cur = e
		default:
			return nil, false
		}
	}
}

// extractHaltingUnit recognizes a block that only halts the test: a
// gotest.Fail call, or a Fatal/Fatalf/FailNow call on a testing.T (FailNow
// also on gotest.T), optionally followed by a bare return that the halt
// makes unreachable.
func extractHaltingUnit(pass *analysis.Pass, body *ast.BlockStmt) *failGuardUnit {
	if body == nil || len(body.List) == 0 || len(body.List) > 2 {
		return nil
	}
	hasReturn := len(body.List) == 2
	if hasReturn {
		ret, ok := body.List[1].(*ast.ReturnStmt)
		if !ok || ret.Results != nil {
			return nil
		}
	}
	es, ok := body.List[0].(*ast.ExprStmt)
	if !ok {
		return nil
	}
	call, ok := es.X.(*ast.CallExpr)
	if !ok {
		return nil
	}

	if assertionFuncName(pass, call.Fun) == "Fail" && len(call.Args) >= 1 {
		return &failGuardUnit{
			tArg:    call.Args[0],
			msgArgs: call.Args[1:],
			qual:    assertionQualifier(call.Fun),
			label:   "Fail",
			halting: true,
		}
	}

	sel, ok := call.Fun.(*ast.SelectorExpr)
	if !ok {
		return nil
	}
	name := sel.Sel.Name
	// Guards on escaped t.T() receivers whose method the t-escape rule
	// covers are that rule's finding; reporting here too would produce
	// overlapping, non-convergent fixes. Methods t-escape does not know
	// (Error) stay with fail-guard.
	if isTMethodCall(sel.X) {
		if _, covered := escapeConfigs[name]; covered {
			return nil
		}
	}
	recvType := pass.TypesInfo.TypeOf(sel.X)
	onTestingT := namedPtrType(recvType, "testing", "T")
	onGotestT := namedPtrType(recvType, gotestImportPath, "T")
	halting := (name == "FailNow" && (onTestingT || onGotestT)) ||
		((name == "Fatal" || name == "Fatalf") && onTestingT)
	weak := (name == "Errorf" && (onTestingT || onGotestT)) ||
		(name == "Error" && onTestingT)
	if !halting && !weak {
		return nil
	}

	// Only files that import gotest have adopted the framework — halting
	// guards elsewhere are idiomatic stdlib Go and out of scope.
	qual, imported := gotestQualifier(pass, call.Pos())
	if !imported {
		return nil
	}
	unit := &failGuardUnit{tArg: sel.X, label: name, halting: halting, returns: hasReturn, qual: qual}
	if name != "FailNow" {
		unit.msgArgs = call.Args
	}
	// A weak call followed by return already stops the enclosing function —
	// converting it would strengthen a helper-local exit into a whole-test
	// halt, so such branches report without a fix and without the
	// "continues" note.
	if !halting && hasReturn {
		unit.forceReport = true
	}
	// t.Fatal is Println-style while msgAndArgs is Printf-style: a single
	// arg survives the translation verbatim, but more would be reinterpreted
	// as format string plus operands — report without a fix.
	if name == "Fatal" && len(call.Args) > 1 {
		unit.forceReport = true
	}
	return unit
}

// spanComments partitions the comments inside the if-statement's span:
// trailing comments on the if line ride along on a rewrite, while a comment
// on any later line makes the rewrite lossy and blocks the fix.
func spanComments(pass *analysis.Pass, ifStmt *ast.IfStmt) (trailing string, interior bool) {
	file := fileContaining(pass, ifStmt.Pos())
	if file == nil {
		return "", false
	}
	ifLine := pass.Fset.Position(ifStmt.Pos()).Line
	var parts []string
	for _, cg := range file.Comments {
		for _, c := range cg.List {
			if c.Pos() <= ifStmt.Pos() || c.End() >= ifStmt.End() {
				continue
			}
			if pass.Fset.Position(c.Pos()).Line == ifLine {
				parts = append(parts, c.Text)
			} else {
				interior = true
			}
		}
	}
	if len(parts) > 0 {
		trailing = " " + strings.Join(parts, " ")
	}
	return trailing, interior
}

// sourceIndent returns the actual leading whitespace of the line holding
// pos, falling back to tabs when the source is unreadable or pos does not
// start its line.
func sourceIndent(fset *token.FileSet, read func(string) []byte, pos token.Pos) string {
	position := fset.Position(pos)
	if content := read(position.Filename); content != nil {
		lineStart := position.Offset - (position.Column - 1)
		if lineStart >= 0 && position.Offset <= len(content) {
			prefix := string(content[lineStart:position.Offset])
			if strings.TrimLeft(prefix, " 	") == "" {
				return prefix
			}
		}
	}
	return strings.Repeat("	", position.Column-1)
}

func namedPtrType(t types.Type, pkgPath, name string) bool {
	ptr, ok := t.(*types.Pointer)
	if !ok {
		return false
	}
	named, ok := ptr.Elem().(*types.Named)
	if !ok {
		return false
	}
	obj := named.Obj()
	return obj.Name() == name && obj.Pkg() != nil && obj.Pkg().Path() == pkgPath
}

// gotestQualifier resolves how the file containing pos refers to the gotest
// package. ok is false when the file does not import it — there is nothing
// to qualify a rewritten assertion with.
func gotestQualifier(pass *analysis.Pass, pos token.Pos) (qual string, ok bool) {
	file := fileContaining(pass, pos)
	if file == nil {
		return "", false
	}
	for _, imp := range file.Imports {
		if strings.Trim(imp.Path.Value, `"`) != gotestImportPath {
			continue
		}
		if imp.Name != nil {
			if imp.Name.Name == "." {
				return "", true
			}
			return imp.Name.Name + ".", true
		}
		return "gotest.", true
	}
	return "", false
}

// splitOr flattens a top-level chain of || operands.
func splitOr(expr ast.Expr) []ast.Expr {
	if paren, ok := expr.(*ast.ParenExpr); ok {
		return splitOr(paren.X)
	}
	if bin, ok := expr.(*ast.BinaryExpr); ok && bin.Op == token.LOR {
		return append(splitOr(bin.X), splitOr(bin.Y)...)
	}
	return []ast.Expr{expr}
}

// msgArgsSafe reports whether every message arg is trivially total to
// evaluate: a basic literal, a plain identifier, or parens around those.
// Anything else — calls, index/selector/deref/type-assertion expressions —
// may have side effects or panic under the assertion's eager evaluation.
func msgArgsSafe(exprs []ast.Expr) bool {
	for _, e := range exprs {
		if !isTotalExpr(e) {
			return false
		}
	}
	return true
}

func isTotalExpr(e ast.Expr) bool {
	switch x := e.(type) {
	case *ast.BasicLit:
		return true
	case *ast.Ident:
		return true
	case *ast.ParenExpr:
		return isTotalExpr(x.X)
	}
	return false
}
