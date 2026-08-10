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
		case "Nil":
			guardNilNotNil(pass, call, false)
		case "NotNil":
			guardNilNotNil(pass, call, true)
		case "Empty":
			guardEmptyNotEmpty(pass, call, false)
		case "NotEmpty":
			guardEmptyNotEmpty(pass, call, true)
		case "ErrorIs":
			guardErrorIs(pass, call)
		case "ErrorContains":
			guardErrorContains(pass, call)
		}
	})
}

// --- True / False ---

// conditionMapping is the outcome of mapping a boolean expression onto a
// stronger assertion: the target assertion name, its arguments after the
// t argument, and a short description of the recognized pattern.
type conditionMapping struct {
	target string
	args   []ast.Expr
	desc   string
}

func simplifyBoolAssertion(pass *analysis.Pass, call *ast.CallExpr, negated bool) {
	if len(call.Args) < 2 {
		return
	}
	m, ok := mapBoolExpr(pass, call.Args[1], negated)
	if !ok {
		return
	}
	source := pick(negated, "False", "True")
	emitSimplify(pass, call, source, m.target, append([]ast.Expr{call.Args[0]}, m.args...), call.Args[2:], m.desc)
}

// mapBoolExpr maps a boolean expression to the assertion that expresses it
// directly. With negated set, the mapping targets the expression's negation
// (the False(t, expr) reading).
func mapBoolExpr(pass *analysis.Pass, expr ast.Expr, negated bool) (conditionMapping, bool) {
	switch e := expr.(type) {
	case *ast.ParenExpr:
		return mapBoolExpr(pass, e.X, negated)

	case *ast.UnaryExpr:
		if e.Op != token.NOT {
			return conditionMapping{}, false
		}
		// !X asserted is X asserted with flipped polarity.
		if m, ok := mapBoolExpr(pass, e.X, !negated); ok {
			return m, true
		}
		return conditionMapping{pick(negated, "True", "False"), []ast.Expr{e.X}, "negation"}, true

	case *ast.BinaryExpr:
		return mapBoolBinary(pass, e, negated)

	case *ast.CallExpr:
		return mapBoolCall(pass, e, negated)
	}
	return conditionMapping{}, false
}

func mapBoolBinary(pass *analysis.Pass, bin *ast.BinaryExpr, negated bool) (conditionMapping, bool) {
	left, right := bin.X, bin.Y

	switch bin.Op {
	case token.EQL:
		if m, ok, handled := mapNilComparison(pass, left, right, negated, false); handled {
			return m, ok
		}
		if m, ok := mapLenEqNeq(left, right, negated, false); ok {
			return m, true
		}
		if m, ok := mapEmptyStrEqNeq(pass, left, right, negated, false); ok {
			return m, true
		}
		l, r := constFirst(pass, left, right)
		return conditionMapping{pick(negated, "NotEqual", "Equal"), []ast.Expr{l, r}, "== comparison"}, true

	case token.NEQ:
		if m, ok, handled := mapNilComparison(pass, left, right, negated, true); handled {
			return m, ok
		}
		if m, ok := mapLenEqNeq(left, right, negated, true); ok {
			return m, true
		}
		if m, ok := mapEmptyStrEqNeq(pass, left, right, negated, true); ok {
			return m, true
		}
		l, r := constFirst(pass, left, right)
		return conditionMapping{pick(negated, "Equal", "NotEqual"), []ast.Expr{l, r}, "!= comparison"}, true

	case token.GTR:
		if inner, ok := isLenCall(left); ok && isIntLit(right, 0) {
			return conditionMapping{pick(negated, "Empty", "NotEmpty"), []ast.Expr{inner}, "len > 0 check"}, true
		}
		return conditionMapping{pick(negated, "LessOrEqual", "Greater"), []ast.Expr{left, right}, "> comparison"}, true

	case token.GEQ:
		if inner, ok := isLenCall(left); ok && isIntLit(right, 1) {
			return conditionMapping{pick(negated, "Empty", "NotEmpty"), []ast.Expr{inner}, "len >= 1 check"}, true
		}
		return conditionMapping{pick(negated, "Less", "GreaterOrEqual"), []ast.Expr{left, right}, ">= comparison"}, true

	case token.LSS:
		return conditionMapping{pick(negated, "GreaterOrEqual", "Less"), []ast.Expr{left, right}, "< comparison"}, true

	case token.LEQ:
		return conditionMapping{pick(negated, "Greater", "LessOrEqual"), []ast.Expr{left, right}, "<= comparison"}, true
	}
	return conditionMapping{}, false
}

// mapNilComparison handles comparisons against nil. handled reports that the
// expression is a nil comparison and no other mapping should be attempted,
// even when no assertion fits the operand's type category.
func mapNilComparison(pass *analysis.Pass, left, right ast.Expr, negated, isNeq bool) (m conditionMapping, ok, handled bool) {
	if !isNilIdent(left) && !isNilIdent(right) {
		return conditionMapping{}, false, false
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
		return conditionMapping{pick(!positive, "Error", "NoError"), []ast.Expr{other}, "error nil check"}, true, true
	case isComparableType(pass, other):
		return conditionMapping{pick(!positive, "NotZero", "Zero"), []ast.Expr{other}, "nil check"}, true, true
	case isNonComparableNilableType(pass, other):
		return conditionMapping{pick(!positive, "NotNil", "Nil"), []ast.Expr{other}, "nil check"}, true, true
	}
	return conditionMapping{}, false, true
}

func mapLenEqNeq(left, right ast.Expr, negated, isNeq bool) (conditionMapping, bool) {
	inner, _, other, ok := extractLenSide(left, right)
	if !ok {
		return conditionMapping{}, false
	}

	// len(x) == 0 or 0 == len(x)
	if isIntLit(other, 0) {
		positive := !isNeq
		if negated {
			positive = !positive
		}
		return conditionMapping{pick(!positive, "NotEmpty", "Empty"), []ast.Expr{inner}, "len == 0 check"}, true
	}

	// len(x) == n where n is not 0 — Len fits whenever the asserted reading
	// is the equality: EQL non-negated, or NEQ negated.
	if isNeq == negated {
		return conditionMapping{"Len", []ast.Expr{inner, other}, "len comparison"}, true
	}

	return conditionMapping{}, false
}

// mapEmptyStrEqNeq handles comparisons of string operands against the empty
// string literal, which Empty/NotEmpty express directly.
func mapEmptyStrEqNeq(pass *analysis.Pass, left, right ast.Expr, negated, isNeq bool) (conditionMapping, bool) {
	var other ast.Expr
	switch {
	case isEmptyStringLit(left):
		other = right
	case isEmptyStringLit(right):
		other = left
	default:
		return conditionMapping{}, false
	}
	if !isStringType(pass, other) {
		return conditionMapping{}, false
	}
	positive := !isNeq
	if negated {
		positive = !positive
	}
	return conditionMapping{pick(!positive, "NotEmpty", "Empty"), []ast.Expr{other}, "empty string check"}, true
}

// constFirst orders Equal/NotEqual operands: a lone constant operand —
// literal, negative literal, or named constant — is the expected value and
// belongs in the expected slot.
func constFirst(pass *analysis.Pass, left, right ast.Expr) (ast.Expr, ast.Expr) {
	if isConstExpr(pass, right) && !isConstExpr(pass, left) {
		return right, left
	}
	return left, right
}

func isConstExpr(pass *analysis.Pass, expr ast.Expr) bool {
	tv, ok := pass.TypesInfo.Types[expr]
	return ok && tv.Value != nil
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

func mapBoolCall(pass *analysis.Pass, inner *ast.CallExpr, negated bool) (conditionMapping, bool) {
	if s, sub, ok := isStringsContains(inner); ok {
		return conditionMapping{pick(negated, "NotContains", "Contains"), []ast.Expr{s, sub}, "strings.Contains call"}, true
	}

	if err, target, ok := isErrorsIs(inner); ok {
		if isNilIdent(target) {
			return conditionMapping{pick(negated, "Error", "NoError"), []ast.Expr{err}, "errors.Is nil check"}, true
		}
		if !negated {
			return conditionMapping{"ErrorIs", []ast.Expr{err, target}, "errors.Is call"}, true
		}
		return conditionMapping{}, false
	}

	if re, s, ok := isRegexpMatchString(pass, inner); ok && !negated {
		return conditionMapping{"Regexp", []ast.Expr{re, s}, "MatchString call"}, true
	}

	if a, b, ok := isReflectDeepEqual(inner); ok {
		return conditionMapping{pick(negated, "NotEqual", "Equal"), []ast.Expr{a, b}, "reflect.DeepEqual call"}, true
	}
	return conditionMapping{}, false
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

	// Empty-string literals: Equal(t, "", x) / Equal(t, x, "")
	if isEmptyStringLit(expected) || isEmptyStringLit(actual) {
		other := expected
		if isEmptyStringLit(expected) {
			other = actual
		}
		if isStringType(pass, other) {
			emitSimplify(pass, call, source, pick(negated, "NotEmpty", "Empty"), []ast.Expr{tArg, other}, msgArgs, "empty string comparison")
			return
		}
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
		case isNonComparableNilableType(pass, other):
			emitSimplify(pass, call, source, pick(negated, "NotNil", "Nil"), []ast.Expr{tArg, other}, msgArgs, "nil comparison")
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

// --- Nil / NotNil type guard ---

func guardNilNotNil(pass *analysis.Pass, call *ast.CallExpr, isNot bool) {
	if len(call.Args) < 2 {
		return
	}
	tArg := call.Args[0]
	arg := call.Args[1]
	msgArgs := call.Args[2:]
	source := pick(isNot, "NotNil", "Nil")

	t := pass.TypesInfo.TypeOf(arg)
	if t == nil || isUntypedNil(t) {
		return
	}

	switch {
	case isErrorType(pass, arg):
		target := pick(isNot, "Error", "NoError")
		emitSimplify(pass, call, source, target, []ast.Expr{tArg, arg}, msgArgs, "error nil check")
	case isConcreteComparableNilableType(pass, arg):
		target := pick(isNot, "NotZero", "Zero")
		emitSimplify(pass, call, source, target, []ast.Expr{tArg, arg}, msgArgs, "nil check")
	case isNonComparableNilableType(pass, arg):
		// OK — this is the intended use of Nil/NotNil
	default:
		report(pass, AssertionTypeGuard, call.Pos(),
			"type %s is not nilable; for zero-value checks, use %s",
			t, pick(isNot, "NotZero", "Zero"))
	}
}

// --- Empty / NotEmpty type guard ---

func guardEmptyNotEmpty(pass *analysis.Pass, call *ast.CallExpr, isNot bool) {
	if len(call.Args) < 2 {
		return
	}
	arg := call.Args[1]
	source := pick(isNot, "NotEmpty", "Empty")

	t := pass.TypesInfo.TypeOf(arg)
	if t == nil || isUntypedNil(t) {
		return
	}

	switch {
	case isErrorType(pass, arg):
		tArg := call.Args[0]
		msgArgs := call.Args[2:]
		target := pick(isNot, "Error", "NoError")
		emitSimplify(pass, call, source, target, []ast.Expr{tArg, arg}, msgArgs, "error empty check")
	case isEmptyableType(pass, arg):
		// OK
	case isPointerType(pass, arg):
		// OK — isEmpty recursively dereferences pointers
	default:
		report(pass, AssertionTypeGuard, call.Pos(),
			"type %s cannot be empty; for nil checks, use %s; for zero-value checks, use %s",
			t, pick(isNot, "NotNil", "Nil"), pick(isNot, "NotZero", "Zero"))
	}
}

// --- ErrorIs type guard ---

func guardErrorIs(pass *analysis.Pass, call *ast.CallExpr) {
	if len(call.Args) < 3 {
		return
	}
	if !isNilIdent(call.Args[2]) {
		return
	}
	tArg := call.Args[0]
	err := call.Args[1]
	msgArgs := call.Args[3:]
	emitSimplify(pass, call, "ErrorIs", "NoError", []ast.Expr{tArg, err}, msgArgs, "nil target")
}

// --- ErrorContains type guard ---

func guardErrorContains(pass *analysis.Pass, call *ast.CallExpr) {
	if len(call.Args) < 3 {
		return
	}
	if !isEmptyStringLit(call.Args[2]) {
		return
	}
	tArg := call.Args[0]
	err := call.Args[1]
	msgArgs := call.Args[3:]
	emitSimplify(pass, call, "ErrorContains", "Error", []ast.Expr{tArg, err}, msgArgs, "empty contains string")
}

// --- reporting ---

func emitSimplify(pass *analysis.Pass, call *ast.CallExpr, from, to string, newArgs, msgArgs []ast.Expr, desc string) {
	qual := assertionQualifier(call.Fun)
	newText := renderAssertion(pass.Fset, qual, to, newArgs, msgArgs)

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

func renderAssertion(fset *token.FileSet, qual, target string, args, msgArgs []ast.Expr) string {
	var parts []string
	for _, arg := range args {
		parts = append(parts, renderExpr(fset, arg))
	}
	for _, arg := range msgArgs {
		parts = append(parts, renderExpr(fset, arg))
	}
	return qual + target + "(" + strings.Join(parts, ", ") + ")"
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

func isEmptyStringLit(expr ast.Expr) bool {
	lit, ok := expr.(*ast.BasicLit)
	return ok && lit.Kind == token.STRING && (lit.Value == `""` || lit.Value == "``")
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
	if !namedPtrType(pass.TypesInfo.TypeOf(sel.X), "regexp", "Regexp") {
		return nil, nil, false
	}
	return sel.X, call.Args[0], true
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
	switch u := t.Underlying().(type) {
	case *types.Slice, *types.Map, *types.Chan, *types.Array:
		return true
	case *types.Basic:
		return u.Kind() == types.String
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

func isNonComparableNilableType(pass *analysis.Pass, expr ast.Expr) bool {
	t := pass.TypesInfo.TypeOf(expr)
	if t == nil || isUntypedNil(t) {
		return false
	}
	switch t.Underlying().(type) {
	case *types.Slice, *types.Map, *types.Signature:
		return true
	}
	return false
}

func isConcreteComparableNilableType(pass *analysis.Pass, expr ast.Expr) bool {
	t := pass.TypesInfo.TypeOf(expr)
	if t == nil || isUntypedNil(t) {
		return false
	}
	switch t.Underlying().(type) {
	case *types.Pointer, *types.Chan:
		return true
	case *types.Interface:
		return true
	}
	return false
}

func isStringType(pass *analysis.Pass, expr ast.Expr) bool {
	t := pass.TypesInfo.TypeOf(expr)
	if t == nil {
		return false
	}
	b, ok := t.Underlying().(*types.Basic)
	return ok && b.Info()&types.IsString != 0
}

func isPointerType(pass *analysis.Pass, expr ast.Expr) bool {
	t := pass.TypesInfo.TypeOf(expr)
	if t == nil {
		return false
	}
	_, ok := t.Underlying().(*types.Pointer)
	return ok
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
