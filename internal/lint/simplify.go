package lint

import (
	"bytes"
	"fmt"
	"go/ast"
	"go/format"
	"go/token"
	"go/types"
	"strings"

	"golang.org/x/tools/go/analysis"
	"golang.org/x/tools/go/ast/inspector"
)

func checkAssertionSimplify(pass *analysis.Pass, insp *inspector.Inspector) {
	insp.Preorder([]ast.Node{(*ast.CallExpr)(nil)}, func(n ast.Node) {
		call := n.(*ast.CallExpr)
		name := resolveAssertionName(call.Fun)
		if name == "" {
			return
		}

		switch name {
		case "True":
			simplifyBoolAssertion(pass, call, false)
		case "False":
			simplifyBoolAssertion(pass, call, true)
		case "Equal":
			simplifyEquality(pass, call, false)
		case "NotEqual":
			simplifyEquality(pass, call, true)
		case "Len":
			simplifyLen(pass, call)
		case "Greater":
			simplifyComparisonLen(pass, call, "Greater", 0)
		case "GreaterOrEqual":
			simplifyComparisonLen(pass, call, "GreaterOrEqual", 1)
		case "Zero":
			simplifyZeroNotZero(pass, call, false)
		case "NotZero":
			simplifyZeroNotZero(pass, call, true)
		case "Contains":
			simplifyContainsErrMsg(pass, call)
		}
	})
}

// --- True / False ---

func simplifyBoolAssertion(pass *analysis.Pass, call *ast.CallExpr, negated bool) {
	if len(call.Args) < 2 {
		return
	}
	tArg := call.Args[0]
	expr := call.Args[1]
	msgArgs := call.Args[2:]
	source := "True"
	if negated {
		source = "False"
	}

	switch e := expr.(type) {
	case *ast.UnaryExpr:
		if e.Op != token.NOT {
			return
		}
		if s, sub, ok := isStringsContains(e.X); ok {
			target := "NotContains"
			if negated {
				target = "Contains"
			}
			emitSimplify(pass, call, source, target, []ast.Expr{tArg, s, sub}, msgArgs, "negated strings.Contains call")
			return
		}
		target := "False"
		if negated {
			target = "True"
		}
		emitSimplify(pass, call, source, target, []ast.Expr{tArg, e.X}, msgArgs, "negation")

	case *ast.BinaryExpr:
		simplifyBoolBinary(pass, call, e, tArg, msgArgs, negated, source)

	case *ast.CallExpr:
		simplifyBoolCall(pass, call, e, tArg, msgArgs, negated, source)
	}
}

func simplifyBoolBinary(pass *analysis.Pass, call *ast.CallExpr, bin *ast.BinaryExpr, tArg ast.Expr, msgArgs []ast.Expr, negated bool, source string) {
	left, right := bin.X, bin.Y

	switch bin.Op {
	case token.EQL:
		if simplifyNilComparison(pass, call, left, right, tArg, msgArgs, negated, source, false) {
			return
		}
		if simplifyLenEqNeq(pass, call, left, right, tArg, msgArgs, negated, source, false) {
			return
		}
		target := pick(negated, "NotEqual", "Equal")
		emitSimplify(pass, call, source, target, []ast.Expr{tArg, left, right}, msgArgs, "== comparison")

	case token.NEQ:
		if simplifyNilComparison(pass, call, left, right, tArg, msgArgs, negated, source, true) {
			return
		}
		if simplifyLenEqNeq(pass, call, left, right, tArg, msgArgs, negated, source, true) {
			return
		}
		target := pick(negated, "Equal", "NotEqual")
		emitSimplify(pass, call, source, target, []ast.Expr{tArg, left, right}, msgArgs, "!= comparison")

	case token.GTR:
		if inner, ok := isLenCall(left); ok && isIntLit(right, 0) {
			emitSimplify(pass, call, source, pick(negated, "Empty", "NotEmpty"), []ast.Expr{tArg, inner}, msgArgs, "len > 0 check")
			return
		}
		emitSimplify(pass, call, source, pick(negated, "LessOrEqual", "Greater"), []ast.Expr{tArg, left, right}, msgArgs, "> comparison")

	case token.GEQ:
		if inner, ok := isLenCall(left); ok && isIntLit(right, 1) {
			emitSimplify(pass, call, source, pick(negated, "Empty", "NotEmpty"), []ast.Expr{tArg, inner}, msgArgs, "len >= 1 check")
			return
		}
		emitSimplify(pass, call, source, pick(negated, "Less", "GreaterOrEqual"), []ast.Expr{tArg, left, right}, msgArgs, ">= comparison")

	case token.LSS:
		emitSimplify(pass, call, source, pick(negated, "GreaterOrEqual", "Less"), []ast.Expr{tArg, left, right}, msgArgs, "< comparison")

	case token.LEQ:
		emitSimplify(pass, call, source, pick(negated, "Greater", "LessOrEqual"), []ast.Expr{tArg, left, right}, msgArgs, "<= comparison")
	}
}

func simplifyNilComparison(pass *analysis.Pass, call *ast.CallExpr, left, right, tArg ast.Expr, msgArgs []ast.Expr, negated bool, source string, isNeq bool) bool {
	if !isNilIdent(left) && !isNilIdent(right) {
		return false
	}
	other := left
	if isNilIdent(left) {
		other = right
	}
	// isNeq flips the polarity (x != nil is the "positive" non-nil assertion)
	positive := !isNeq
	if negated {
		positive = !positive
	}
	switch {
	case isErrorType(pass, other):
		target := "NoError"
		if !positive {
			target = "Error"
		}
		emitSimplify(pass, call, source, target, []ast.Expr{tArg, other}, msgArgs, "error nil check")
	case isComparableType(pass, other):
		target := "Zero"
		if !positive {
			target = "NotZero"
		}
		emitSimplify(pass, call, source, target, []ast.Expr{tArg, other}, msgArgs, "nil check")
	case isEmptyableType(pass, other):
		target := "Empty"
		if !positive {
			target = "NotEmpty"
		}
		emitSimplify(pass, call, source, target, []ast.Expr{tArg, other}, msgArgs, "nil check")
	}
	return true
}

func simplifyLenEqNeq(pass *analysis.Pass, call *ast.CallExpr, left, right, tArg ast.Expr, msgArgs []ast.Expr, negated bool, source string, isNeq bool) bool {
	inner, lenExpr, other, ok := extractLenSide(left, right)
	if !ok {
		return false
	}
	_ = lenExpr

	// len(x) == 0 or 0 == len(x)
	if isIntLit(other, 0) {
		positive := !isNeq
		if negated {
			positive = !positive
		}
		emitSimplify(pass, call, source, pick(!positive, "NotEmpty", "Empty"), []ast.Expr{tArg, inner}, msgArgs, "len == 0 check")
		return true
	}

	// len(x) == n where n is not 0 — suggest Len in non-negated EQL context only
	if !isNeq && !negated {
		emitSimplify(pass, call, source, "Len", []ast.Expr{tArg, inner, other}, msgArgs, "len comparison")
		return true
	}

	return false
}

func extractLenSide(left, right ast.Expr) (inner, lenExpr, other ast.Expr, ok bool) {
	if inner, ok := isLenCall(left); ok {
		return inner, left, right, true
	}
	if inner, ok := isLenCall(right); ok {
		return inner, right, left, true
	}
	return nil, nil, nil, false
}

func simplifyBoolCall(pass *analysis.Pass, call *ast.CallExpr, inner *ast.CallExpr, tArg ast.Expr, msgArgs []ast.Expr, negated bool, source string) {
	if s, sub, ok := isStringsContains(inner); ok {
		emitSimplify(pass, call, source, pick(negated, "NotContains", "Contains"), []ast.Expr{tArg, s, sub}, msgArgs, "strings.Contains call")
		return
	}

	if err, target, ok := isErrorsIs(inner); ok && !negated {
		emitSimplify(pass, call, source, "ErrorIs", []ast.Expr{tArg, err, target}, msgArgs, "errors.Is call")
		return
	}

	if re, s, ok := isRegexpMatchString(pass, inner); ok && !negated {
		emitSimplify(pass, call, source, "Regexp", []ast.Expr{tArg, re, s}, msgArgs, "MatchString call")
		return
	}

	if a, b, ok := isReflectDeepEqual(inner); ok {
		emitSimplify(pass, call, source, pick(negated, "NotEqual", "Equal"), []ast.Expr{tArg, a, b}, msgArgs, "reflect.DeepEqual call")
		return
	}
}

// --- Equal / NotEqual ---

func simplifyEquality(pass *analysis.Pass, call *ast.CallExpr, negated bool) {
	if len(call.Args) < 3 {
		return
	}
	tArg := call.Args[0]
	expected := call.Args[1]
	actual := call.Args[2]
	msgArgs := call.Args[3:]
	source := pick(negated, "NotEqual", "Equal")

	// Bool literals: Equal(t, true, x) / Equal(t, x, true)
	if v, expr, ok := extractBoolLiteral(expected, actual); ok {
		positive := v != negated // true+Equal or false+NotEqual → True; otherwise False
		emitSimplify(pass, call, source, pick(!positive, "False", "True"), []ast.Expr{tArg, expr}, msgArgs, "bool literal comparison")
		return
	}

	// Nil literals: Equal(t, nil, x) / Equal(t, x, nil)
	if isNilIdent(expected) || isNilIdent(actual) {
		other := expected
		if isNilIdent(expected) {
			other = actual
		}
		switch {
		case isErrorType(pass, other):
			emitSimplify(pass, call, source, pick(negated, "Error", "NoError"), []ast.Expr{tArg, other}, msgArgs, "nil error comparison")
		case isComparableType(pass, other):
			emitSimplify(pass, call, source, pick(negated, "NotZero", "Zero"), []ast.Expr{tArg, other}, msgArgs, "nil comparison")
		case isEmptyableType(pass, other):
			emitSimplify(pass, call, source, pick(negated, "NotEmpty", "Empty"), []ast.Expr{tArg, other}, msgArgs, "nil comparison")
		}
		return
	}

	// Len calls: Equal(t, len(x), n) / Equal(t, n, len(x))
	inner, _, other, ok := extractLenSide(expected, actual)
	if !ok {
		return
	}
	if isIntLit(other, 0) {
		emitSimplify(pass, call, source, pick(negated, "NotEmpty", "Empty"), []ast.Expr{tArg, inner}, msgArgs, "len == 0 comparison")
		return
	}
	if !negated {
		emitSimplify(pass, call, source, "Len", []ast.Expr{tArg, inner, other}, msgArgs, "len comparison")
	}
}

// --- Len ---

func simplifyLen(pass *analysis.Pass, call *ast.CallExpr) {
	if len(call.Args) < 3 {
		return
	}
	if isIntLit(call.Args[2], 0) && !isNilIdent(call.Args[1]) {
		emitSimplify(pass, call, "Len", "Empty", []ast.Expr{call.Args[0], call.Args[1]}, call.Args[3:], "zero length check")
	}
}

// --- Greater / GreaterOrEqual ---

func simplifyComparisonLen(pass *analysis.Pass, call *ast.CallExpr, source string, threshold int) {
	if len(call.Args) < 3 {
		return
	}
	inner, ok := isLenCall(call.Args[1])
	if !ok {
		return
	}
	if isIntLit(call.Args[2], threshold) {
		desc := "len > 0 check"
		if threshold == 1 {
			desc = "len >= 1 check"
		}
		emitSimplify(pass, call, source, "NotEmpty", []ast.Expr{call.Args[0], inner}, call.Args[3:], desc)
	}
}

// --- Zero / NotZero ---

func simplifyZeroNotZero(pass *analysis.Pass, call *ast.CallExpr, isNotZero bool) {
	if len(call.Args) < 2 {
		return
	}
	if !isErrorType(pass, call.Args[1]) {
		return
	}
	source := pick(isNotZero, "NotZero", "Zero")
	target := pick(isNotZero, "Error", "NoError")
	emitSimplify(pass, call, source, target, []ast.Expr{call.Args[0], call.Args[1]}, call.Args[2:], "error zero check")
}

// --- Contains ---

func simplifyContainsErrMsg(pass *analysis.Pass, call *ast.CallExpr) {
	if len(call.Args) < 3 {
		return
	}
	recv, ok := isErrorMethodCall(call.Args[1])
	if !ok {
		return
	}
	emitSimplify(pass, call, "Contains", "ErrorContains", []ast.Expr{call.Args[0], recv, call.Args[2]}, call.Args[3:], "err.Error() contains check")
}

// --- reporting ---

func emitSimplify(pass *analysis.Pass, call *ast.CallExpr, from, to string, newArgs, msgArgs []ast.Expr, desc string) {
	qual := assertionQualifier(call.Fun)

	var parts []string
	for _, arg := range newArgs {
		parts = append(parts, renderExpr(pass.Fset, arg))
	}
	for _, arg := range msgArgs {
		parts = append(parts, renderExpr(pass.Fset, arg))
	}
	newText := qual + to + "(" + strings.Join(parts, ", ") + ")"

	reportWithFix(pass, AssertionSimplify, call.Pos(),
		[]analysis.SuggestedFix{{
			Message: fmt.Sprintf("use %s%s", qual, to),
			TextEdits: []analysis.TextEdit{{
				Pos:     call.Pos(),
				End:     call.End(),
				NewText: []byte(newText),
			}},
		}},
		"use %s instead of %s for %s", to, from, desc)
}

// --- expression helpers ---

func isNilIdent(expr ast.Expr) bool {
	id, ok := expr.(*ast.Ident)
	return ok && id.Name == "nil"
}

func isBoolIdent(expr ast.Expr) (val bool, ok bool) {
	id, ok := expr.(*ast.Ident)
	if !ok {
		return false, false
	}
	switch id.Name {
	case "true":
		return true, true
	case "false":
		return false, true
	}
	return false, false
}

func extractBoolLiteral(a, b ast.Expr) (val bool, other ast.Expr, ok bool) {
	if v, ok := isBoolIdent(a); ok {
		return v, b, true
	}
	if v, ok := isBoolIdent(b); ok {
		return v, a, true
	}
	return false, nil, false
}

func isIntLit(expr ast.Expr, want int) bool {
	lit, ok := expr.(*ast.BasicLit)
	if !ok || lit.Kind != token.INT {
		return false
	}
	return lit.Value == fmt.Sprintf("%d", want)
}

func isLenCall(expr ast.Expr) (inner ast.Expr, ok bool) {
	call, ok := expr.(*ast.CallExpr)
	if !ok || len(call.Args) != 1 {
		return nil, false
	}
	id, ok := call.Fun.(*ast.Ident)
	if !ok || id.Name != "len" {
		return nil, false
	}
	return call.Args[0], true
}

func isStringsContains(expr ast.Expr) (s, sub ast.Expr, ok bool) {
	call, ok := expr.(*ast.CallExpr)
	if !ok || len(call.Args) != 2 {
		return nil, nil, false
	}
	sel, ok := call.Fun.(*ast.SelectorExpr)
	if !ok || sel.Sel.Name != "Contains" {
		return nil, nil, false
	}
	id, ok := sel.X.(*ast.Ident)
	if !ok || id.Name != "strings" {
		return nil, nil, false
	}
	return call.Args[0], call.Args[1], true
}

func isErrorsIs(expr ast.Expr) (err, target ast.Expr, ok bool) {
	call, ok := expr.(*ast.CallExpr)
	if !ok || len(call.Args) != 2 {
		return nil, nil, false
	}
	sel, ok := call.Fun.(*ast.SelectorExpr)
	if !ok || sel.Sel.Name != "Is" {
		return nil, nil, false
	}
	id, ok := sel.X.(*ast.Ident)
	if !ok || id.Name != "errors" {
		return nil, nil, false
	}
	return call.Args[0], call.Args[1], true
}

func isRegexpMatchString(pass *analysis.Pass, expr ast.Expr) (re, s ast.Expr, ok bool) {
	call, ok := expr.(*ast.CallExpr)
	if !ok || len(call.Args) != 1 {
		return nil, nil, false
	}
	sel, ok := call.Fun.(*ast.SelectorExpr)
	if !ok || sel.Sel.Name != "MatchString" {
		return nil, nil, false
	}
	t := pass.TypesInfo.TypeOf(sel.X)
	if t == nil || !isRegexpType(t) {
		return nil, nil, false
	}
	return sel.X, call.Args[0], true
}

func isRegexpType(t types.Type) bool {
	ptr, ok := t.(*types.Pointer)
	if !ok {
		return false
	}
	named, ok := ptr.Elem().(*types.Named)
	if !ok {
		return false
	}
	obj := named.Obj()
	return obj.Name() == "Regexp" && obj.Pkg() != nil && obj.Pkg().Path() == "regexp"
}

func isReflectDeepEqual(expr ast.Expr) (a, b ast.Expr, ok bool) {
	call, ok := expr.(*ast.CallExpr)
	if !ok || len(call.Args) != 2 {
		return nil, nil, false
	}
	sel, ok := call.Fun.(*ast.SelectorExpr)
	if !ok || sel.Sel.Name != "DeepEqual" {
		return nil, nil, false
	}
	id, ok := sel.X.(*ast.Ident)
	if !ok || id.Name != "reflect" {
		return nil, nil, false
	}
	return call.Args[0], call.Args[1], true
}

func isErrorMethodCall(expr ast.Expr) (recv ast.Expr, ok bool) {
	call, ok := expr.(*ast.CallExpr)
	if !ok || len(call.Args) != 0 {
		return nil, false
	}
	sel, ok := call.Fun.(*ast.SelectorExpr)
	if !ok || sel.Sel.Name != "Error" {
		return nil, false
	}
	return sel.X, true
}

func isEmptyableType(pass *analysis.Pass, expr ast.Expr) bool {
	t := pass.TypesInfo.TypeOf(expr)
	if t == nil {
		return false
	}
	switch t.Underlying().(type) {
	case *types.Slice, *types.Map, *types.Chan, *types.Array:
		return true
	}
	return false
}

func isComparableType(pass *analysis.Pass, expr ast.Expr) bool {
	t := pass.TypesInfo.TypeOf(expr)
	if t == nil || isUntypedNil(t) {
		return false
	}
	return types.Comparable(t)
}

func isErrorType(pass *analysis.Pass, expr ast.Expr) bool {
	t := pass.TypesInfo.TypeOf(expr)
	if t == nil || isUntypedNil(t) {
		return false
	}
	errorType := types.Universe.Lookup("error").Type()
	return types.AssignableTo(t, errorType)
}

func isUntypedNil(t types.Type) bool {
	b, ok := t.(*types.Basic)
	return ok && b.Kind() == types.UntypedNil
}

func assertionQualifier(expr ast.Expr) string {
	switch fn := expr.(type) {
	case *ast.SelectorExpr:
		if id, ok := fn.X.(*ast.Ident); ok {
			return id.Name + "."
		}
	case *ast.IndexExpr:
		return assertionQualifier(fn.X)
	case *ast.IndexListExpr:
		return assertionQualifier(fn.X)
	}
	return ""
}

func renderExpr(fset *token.FileSet, node ast.Expr) string {
	var buf bytes.Buffer
	_ = format.Node(&buf, fset, node)
	return buf.String()
}

func pick(cond bool, ifTrue, ifFalse string) string {
	if cond {
		return ifTrue
	}
	return ifFalse
}
