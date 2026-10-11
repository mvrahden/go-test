package migrate

import (
	"bytes"
	"fmt"
	"go/ast"
	"go/format"
	"go/parser"
	"go/token"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/mvrahden/go-test/internal/about"
	"github.com/mvrahden/go-test/internal/gotestast"
)

var gotestImport = about.Repo + "/pkg/gotest"

// assertionMap maps testify assertion method names to gotest function names.
var assertionMap = map[string]string{
	"Equal":          "Equal",
	"NotEqual":       "NotEqual",
	"NoError":        "NoError",
	"Error":          "Error",
	"ErrorIs":        "ErrorIs",
	"ErrorContains":  "ErrorContains",
	"True":           "True",
	"False":          "False",
	"Nil":            "Nil",
	"NotNil":         "NotNil",
	"Empty":          "Empty",
	"NotEmpty":       "NotEmpty",
	"Len":            "Len",
	"Contains":       "Contains",
	"Zero":           "Zero",
	"Greater":        "Greater",
	"GreaterOrEqual": "GreaterOrEqual",
	"Less":           "Less",
	"LessOrEqual":    "LessOrEqual",
}

// unsupportedHooks are testify lifecycle hooks that have no gotest equivalent
// and cannot be converted automatically.
var unsupportedHooks = map[string]bool{
	"SetupSubTest":    true,
	"TearDownSubTest": true,
	"BeforeTest":      true,
	"AfterTest":       true,
	"HandleStats":     true,
}

// testifyAssertionNames is a superset of testify assertion method names. It is
// used to recognize unconverted direct s.<Method>(...) assertion calls on the
// suite receiver, where the method name alone must identify an assertion.
var testifyAssertionNames = map[string]bool{
	"Condition": true, "DirExists": true, "ElementsMatch": true,
	"EqualError": true, "EqualExportedValues": true, "EqualValues": true,
	"ErrorAs": true, "Eventually": true, "EventuallyWithT": true,
	"Exactly": true, "FileExists": true, "Implements": true,
	"InDelta": true, "InDeltaMapValues": true, "InDeltaSlice": true,
	"InEpsilon": true, "InEpsilonSlice": true, "IsDecreasing": true,
	"IsIncreasing": true, "IsNonDecreasing": true, "IsNonIncreasing": true,
	"IsType": true, "JSONEq": true, "Negative": true, "Never": true,
	"NoDirExists": true, "NoFileExists": true, "NotElementsMatch": true,
	"NotErrorAs": true, "NotErrorIs": true, "NotImplements": true,
	"NotPanics": true, "NotRegexp": true, "NotSame": true,
	"NotSubset": true, "NotZero": true, "Panics": true,
	"PanicsWithError": true, "PanicsWithValue": true, "Positive": true,
	"Regexp": true, "Same": true, "Subset": true,
	"WithinDuration": true, "WithinRange": true, "YAMLEq": true,
}

// isTestifyAssertionName reports whether name is a known testify assertion
// method (including mapped ones and formatted "...f" variants).
func isTestifyAssertionName(name string) bool {
	if testifyAssertionNames[name] || assertionMap[name] != "" {
		return true
	}
	if base, ok := strings.CutSuffix(name, "f"); ok {
		return testifyAssertionNames[base] || assertionMap[base] != ""
	}
	return false
}

// MigrationPlan describes all suites found in a single file.
type MigrationPlan struct {
	Suites []SuiteMigration
}

// SuiteMigration captures the details of a single testify/suite struct to migrate.
type SuiteMigration struct {
	OldName       string
	NewName       string
	SetupSuite    string
	TearDownSuite string
	SetupTest     string
	TearDownTest  string
	TestMethods   []string
	RunnerFunc    string // func name of the suite.Run wrapper
	ReceiverName  string // receiver variable name (e.g., "s")
}

// MigrateResult describes what was migrated in a file, or why it was not.
type MigrateResult struct {
	File    string
	OldName string
	NewName string
	// Markers counts the TODO(gotest-migrate) lines the file was left with,
	// including those an earlier run placed.
	Markers int
	// Refusal names why the file was not migrated at all; markers stand
	// where the reason was found.
	Refusal string
	// RefsOnly marks a file that holds no suite but refers to one a sibling
	// file renamed; only those references were rewritten.
	RefsOnly bool
}

// DeriveNewName computes the new suite name from an old one.
// If the name already ends with "TestSuite", it is left as-is.
// If it ends with "Suite", "Suite" is replaced with "TestSuite".
// Otherwise, "TestSuite" is appended.
func DeriveNewName(old string) string {
	if strings.HasSuffix(old, "TestSuite") {
		return old
	}
	if strings.HasSuffix(old, "Suite") {
		return old[:len(old)-len("Suite")] + "TestSuite"
	}
	return old + "TestSuite"
}

// AnalyzeFile inspects a parsed Go file and returns a migration plan.
func AnalyzeFile(f *ast.File) MigrationPlan {
	plan := MigrationPlan{}

	// Step 1: Find all structs embedding suite.Suite
	suiteMap := map[string]*SuiteMigration{} // keyed by struct name
	for _, decl := range f.Decls {
		gd, ok := decl.(*ast.GenDecl)
		if !ok || gd.Tok != token.TYPE {
			continue
		}
		for _, spec := range gd.Specs {
			ts, ok := spec.(*ast.TypeSpec)
			if !ok {
				continue
			}
			st, ok := ts.Type.(*ast.StructType)
			if !ok || st.Fields == nil {
				continue
			}
			for _, field := range st.Fields.List {
				if isSuiteSuiteField(field) {
					sm := &SuiteMigration{
						OldName: ts.Name.Name,
						NewName: DeriveNewName(ts.Name.Name),
					}
					suiteMap[ts.Name.Name] = sm
					break
				}
			}
		}
	}

	if len(suiteMap) == 0 {
		return plan
	}

	// Step 2: Find methods on those suite structs
	for _, decl := range f.Decls {
		fd, ok := decl.(*ast.FuncDecl)
		if !ok || fd.Recv == nil || len(fd.Recv.List) == 0 {
			continue
		}

		recvTypeName, recvVarName := extractReceiverInfo(fd.Recv.List[0])
		sm, exists := suiteMap[recvTypeName]
		if !exists {
			continue
		}

		if sm.ReceiverName == "" {
			sm.ReceiverName = recvVarName
		}

		name := fd.Name.Name
		switch name {
		case "SetupSuite":
			sm.SetupSuite = name
		case "TearDownSuite":
			sm.TearDownSuite = name
		case "SetupTest":
			sm.SetupTest = name
		case "TearDownTest":
			sm.TearDownTest = name
		default:
			if strings.HasPrefix(name, "Test") {
				sm.TestMethods = append(sm.TestMethods, name)
			}
		}
	}

	// Step 3: Find runner functions: func Test*(t *testing.T) { suite.Run(t, ...) }
	for _, decl := range f.Decls {
		fd, ok := decl.(*ast.FuncDecl)
		if !ok || fd.Recv != nil {
			continue
		}
		if !strings.HasPrefix(fd.Name.Name, "Test") {
			continue
		}
		if !hasTestingTParam(fd) {
			continue
		}
		if suiteRunTarget := findSuiteRunTarget(fd); suiteRunTarget != "" {
			if sm, exists := suiteMap[suiteRunTarget]; exists {
				sm.RunnerFunc = fd.Name.Name
			}
		}
	}

	for _, sm := range suiteMap {
		plan.Suites = append(plan.Suites, *sm)
	}
	return plan
}

// isSuiteSuiteField checks if a struct field is an anonymous embedding of suite.Suite.
func isSuiteSuiteField(field *ast.Field) bool {
	// Anonymous field: no names
	if len(field.Names) != 0 {
		return false
	}
	sel, ok := field.Type.(*ast.SelectorExpr)
	if !ok {
		return false
	}
	ident, ok := sel.X.(*ast.Ident)
	if !ok {
		return false
	}
	return ident.Name == "suite" && sel.Sel.Name == "Suite"
}

// extractReceiverInfo returns the type name and variable name from a receiver field.
func extractReceiverInfo(field *ast.Field) (typeName, varName string) {
	if len(field.Names) > 0 {
		varName = field.Names[0].Name
	}
	typeName = gotestast.ReceiverTypeName(field.Type)
	return
}

// hasTestingTParam checks if a func has a single parameter of type *testing.T.
func hasTestingTParam(fd *ast.FuncDecl) bool {
	if fd.Type.Params == nil || len(fd.Type.Params.List) != 1 {
		return false
	}
	p := fd.Type.Params.List[0]
	star, ok := p.Type.(*ast.StarExpr)
	if !ok {
		return false
	}
	sel, ok := star.X.(*ast.SelectorExpr)
	if !ok {
		return false
	}
	ident, ok := sel.X.(*ast.Ident)
	if !ok {
		return false
	}
	return ident.Name == "testing" && sel.Sel.Name == "T"
}

// findSuiteRunTarget checks if a function body calls suite.Run(t, new(X)) or
// suite.Run(t, &X{}) and returns X.
func findSuiteRunTarget(fd *ast.FuncDecl) string {
	if fd.Body == nil {
		return ""
	}
	for _, stmt := range fd.Body.List {
		es, ok := stmt.(*ast.ExprStmt)
		if !ok {
			continue
		}
		call, ok := es.X.(*ast.CallExpr)
		if !ok {
			continue
		}
		// Check for suite.Run(...)
		sel, ok := call.Fun.(*ast.SelectorExpr)
		if !ok {
			continue
		}
		ident, ok := sel.X.(*ast.Ident)
		if !ok || ident.Name != "suite" || sel.Sel.Name != "Run" {
			continue
		}
		if len(call.Args) != 2 {
			continue
		}
		// Extract type from second arg: new(X) or &X{}
		return extractSuiteType(call.Args[1])
	}
	return ""
}

// extractSuiteType extracts the suite type name from new(X) or &X{}.
func extractSuiteType(expr ast.Expr) string {
	switch e := expr.(type) {
	case *ast.CallExpr:
		// new(X)
		if ident, ok := e.Fun.(*ast.Ident); ok && ident.Name == "new" {
			if len(e.Args) == 1 {
				if typeIdent, ok := e.Args[0].(*ast.Ident); ok {
					return typeIdent.Name
				}
			}
		}
	case *ast.UnaryExpr:
		// &X{}
		if comp, ok := e.X.(*ast.CompositeLit); ok {
			if typeIdent, ok := comp.Type.(*ast.Ident); ok {
				return typeIdent.Name
			}
		}
	}
	return ""
}

// TransformFile applies the migration transformations to the AST in-place.
func TransformFile(fset *token.FileSet, f *ast.File, plan MigrationPlan) {
	transformFile(f, plan, nil)
}

// transformFile is TransformFile that also renames the suites of sibling
// files (old name to new) wherever f refers to them.
func transformFile(f *ast.File, plan MigrationPlan, siblings map[string]string) {
	if len(plan.Suites) == 0 {
		return
	}

	// Build lookup maps
	oldToNew := map[string]string{}
	suiteReceivers := map[string]string{} // old name -> receiver var
	runnerFuncs := map[string]bool{}
	lifecycleMethods := map[string]bool{}
	suiteOldNames := map[string]bool{}

	for i := range plan.Suites {
		sm := &plan.Suites[i]
		oldToNew[sm.OldName] = sm.NewName
		suiteReceivers[sm.OldName] = sm.ReceiverName
		suiteOldNames[sm.OldName] = true
		if sm.RunnerFunc != "" {
			runnerFuncs[sm.RunnerFunc] = true
		}
	}

	lifecycleRenames := map[string]string{
		"SetupSuite":    "BeforeAll",
		"TearDownSuite": "AfterAll",
		"SetupTest":     "BeforeEach",
		"TearDownTest":  "AfterEach",
	}

	for k := range lifecycleRenames {
		lifecycleMethods[k] = true
	}

	// 1. Remove runner functions
	newDecls := make([]ast.Decl, 0, len(f.Decls))
	for _, decl := range f.Decls {
		if fd, ok := decl.(*ast.FuncDecl); ok && fd.Recv == nil {
			if runnerFuncs[fd.Name.Name] {
				continue // skip runner func
			}
		}
		newDecls = append(newDecls, decl)
	}
	f.Decls = newDecls

	// 2. Transform structs and methods
	for _, decl := range f.Decls {
		switch d := decl.(type) {
		case *ast.GenDecl:
			if d.Tok == token.TYPE {
				for _, spec := range d.Specs {
					ts, ok := spec.(*ast.TypeSpec)
					if !ok {
						continue
					}
					if newName, exists := oldToNew[ts.Name.Name]; exists {
						// Rename struct
						ts.Name.Name = newName

						// Remove suite.Suite embedding
						if st, ok := ts.Type.(*ast.StructType); ok && st.Fields != nil {
							filtered := make([]*ast.Field, 0, len(st.Fields.List))
							for _, field := range st.Fields.List {
								if !isSuiteSuiteField(field) {
									filtered = append(filtered, field)
								}
							}
							st.Fields.List = filtered
						}
					}
				}
			}

		case *ast.FuncDecl:
			vars, onSuite := suiteVars(d, suiteOldNames)
			if len(vars) == 0 {
				continue
			}

			if onSuite {
				// Rename receiver type
				recvTypeName, _ := extractReceiverInfo(d.Recv.List[0])
				renameReceiverType(d.Recv.List[0], oldToNew[recvTypeName])

				// Rename lifecycle methods and add t parameter
				if newName, ok := lifecycleRenames[d.Name.Name]; ok {
					d.Name.Name = newName
					addGotestTParam(d)
				}

				// Test methods: add t parameter
				if strings.HasPrefix(d.Name.Name, "Test") {
					addGotestTParam(d)
				}
			}

			// Rewrite assertions only where t is in scope, and s.T() only
			// where t is a *gotest.T; the rest is left for a marker.
			kind := tParamKind(d)
			for _, v := range vars {
				if d.Body != nil && kind != "" {
					rewriteAssertions(d.Body, v, assertionMap)
				}
				if d.Body != nil && kind == "gotest" {
					rewriteSTCalls(d.Body, v)
				}
			}
		}
	}

	// 3. Rename every other reference to a renamed suite type
	for old, nn := range siblings {
		oldToNew[old] = nn
	}
	renameTypeRefs(f, oldToNew)

	// 4. Rewrite imports
	rewriteImports(f)
}

// renameTypeRefs renames every identifier that names a key of renames —
// parameter and variable types, conversions, composite literals — and
// reports whether it renamed any. Selector, field and func names are not
// type references and stay.
func renameTypeRefs(f *ast.File, renames map[string]string) bool {
	if len(renames) == 0 {
		return false
	}
	skip := map[*ast.Ident]bool{}
	ast.Inspect(f, func(n ast.Node) bool {
		switch x := n.(type) {
		case *ast.SelectorExpr:
			skip[x.Sel] = true
		case *ast.Field:
			for _, name := range x.Names {
				skip[name] = true
			}
		case *ast.FuncDecl:
			skip[x.Name] = true
		}
		return true
	})
	renamed := false
	ast.Inspect(f, func(n ast.Node) bool {
		ident, ok := n.(*ast.Ident)
		if !ok || skip[ident] {
			return true
		}
		if nn, ok := renames[ident.Name]; ok && nn != ident.Name {
			ident.Name = nn
			renamed = true
		}
		return true
	})
	return renamed
}

// suiteVars names fd's variables of a suite type: a suite receiver (onSuite)
// and any parameter of one, as a free helper taking the suite has.
func suiteVars(fd *ast.FuncDecl, suites map[string]bool) (vars []string, onSuite bool) {
	if fd.Recv != nil && len(fd.Recv.List) > 0 {
		typeName, varName := extractReceiverInfo(fd.Recv.List[0])
		if suites[typeName] {
			onSuite = true
			if varName != "" && varName != "_" {
				vars = append(vars, varName)
			}
		}
	}
	if fd.Type.Params == nil {
		return vars, onSuite
	}
	for _, p := range fd.Type.Params.List {
		if !suites[gotestast.ReceiverTypeName(p.Type)] {
			continue
		}
		for _, name := range p.Names {
			if name.Name != "_" {
				vars = append(vars, name.Name)
			}
		}
	}
	return vars, onSuite
}

// renameReceiverType changes the type name in a method receiver.
func renameReceiverType(field *ast.Field, newName string) {
	switch t := field.Type.(type) {
	case *ast.StarExpr:
		if ident, ok := t.X.(*ast.Ident); ok {
			ident.Name = newName
		}
	case *ast.Ident:
		t.Name = newName
	}
}

// addGotestTParam adds `t *gotest.T` as the first parameter of a function.
func addGotestTParam(fd *ast.FuncDecl) {
	tParam := &ast.Field{
		Names: []*ast.Ident{ast.NewIdent("t")},
		Type: &ast.StarExpr{
			X: &ast.SelectorExpr{
				X:   ast.NewIdent("gotest"),
				Sel: ast.NewIdent("T"),
			},
		},
	}
	if fd.Type.Params == nil {
		fd.Type.Params = &ast.FieldList{}
	}
	fd.Type.Params.List = append([]*ast.Field{tParam}, fd.Type.Params.List...)
}

// rewriteAssertions rewrites testify assertion calls in a block statement.
func rewriteAssertions(body *ast.BlockStmt, recvName string, assertionMap map[string]string) {
	for i, stmt := range body.List {
		es, ok := stmt.(*ast.ExprStmt)
		if !ok {
			continue
		}

		call, ok := es.X.(*ast.CallExpr)
		if !ok {
			continue
		}

		if newExpr := rewriteAssertionCall(call, recvName, assertionMap); newExpr != nil {
			es.X = newExpr
		}

		// Also check for nested blocks (if/for/etc.)
		body.List[i] = stmt
	}

	// Walk nested blocks
	ast.Inspect(body, func(n ast.Node) bool {
		if node, ok := n.(*ast.BlockStmt); ok {
			if node != body {
				rewriteAssertions(node, recvName, assertionMap)
				return false
			}
		}
		return true
	})
}

// rewriteAssertionCall attempts to rewrite a single assertion call.
// Returns nil if no rewrite is applicable.
func rewriteAssertionCall(call *ast.CallExpr, recvName string, assertionMap map[string]string) ast.Expr {
	// Pattern 1: s.Require().Method(args...) or s.Assert().Method(args...)
	if sel, ok := call.Fun.(*ast.SelectorExpr); ok {
		methodName := sel.Sel.Name
		gotestFunc, mapped := assertionMap[methodName]
		if !mapped {
			return nil
		}

		if innerCall, ok := sel.X.(*ast.CallExpr); ok {
			if innerSel, ok := innerCall.Fun.(*ast.SelectorExpr); ok {
				if ident, ok := innerSel.X.(*ast.Ident); ok && ident.Name == recvName {
					if innerSel.Sel.Name == "Require" || innerSel.Sel.Name == "Assert" {
						// Rewrite to gotest.Func(t, args...)
						return makeGotestCall(gotestFunc, call.Args)
					}
				}
			}
		}
	}

	// Pattern 2: assert.Method(s.T(), args...) or require.Method(s.T(), args...)
	if sel, ok := call.Fun.(*ast.SelectorExpr); ok {
		if pkgIdent, ok := sel.X.(*ast.Ident); ok {
			if pkgIdent.Name == "assert" || pkgIdent.Name == "require" {
				methodName := sel.Sel.Name
				gotestFunc, mapped := assertionMap[methodName]
				if !mapped {
					return nil
				}
				if len(call.Args) >= 1 && isSTCall(call.Args[0], recvName) {
					// Remove the s.T() first arg, replace with t
					remainingArgs := call.Args[1:]
					return makeGotestCall(gotestFunc, remainingArgs)
				}
			}
		}
	}

	return nil
}

// isSTCall checks if an expression is s.T() where s is the receiver name.
func isSTCall(expr ast.Expr, recvName string) bool {
	call, ok := expr.(*ast.CallExpr)
	if !ok {
		return false
	}
	sel, ok := call.Fun.(*ast.SelectorExpr)
	if !ok {
		return false
	}
	ident, ok := sel.X.(*ast.Ident)
	if !ok {
		return false
	}
	return ident.Name == recvName && sel.Sel.Name == "T" && len(call.Args) == 0
}

// makeGotestCall creates a gotest.Func(t, args...) call expression.
func makeGotestCall(funcName string, args []ast.Expr) *ast.CallExpr {
	newArgs := make([]ast.Expr, 0, len(args)+1)
	newArgs = append(newArgs, ast.NewIdent("t"))
	newArgs = append(newArgs, args...)

	return &ast.CallExpr{
		Fun: &ast.SelectorExpr{
			X:   ast.NewIdent("gotest"),
			Sel: ast.NewIdent(funcName),
		},
		Args: newArgs,
	}
}

// rewriteImports removes the testify and testing imports nothing references
// any more and adds the gotest import. A plain *testing.T helper may still
// call assert.X, so an import is dropped only once it is unused.
func rewriteImports(f *ast.File) {
	removable := map[string]bool{
		"github.com/stretchr/testify/suite":   true,
		"github.com/stretchr/testify/assert":  true,
		"github.com/stretchr/testify/require": true,
		"testing":                             true,
	}

	// Package names still qualifying a selector after the rewrite.
	used := map[string]bool{}
	ast.Inspect(f, func(n ast.Node) bool {
		if sel, ok := n.(*ast.SelectorExpr); ok {
			if ident, ok := sel.X.(*ast.Ident); ok {
				used[ident.Name] = true
			}
		}
		return true
	})

	for _, decl := range f.Decls {
		gd, ok := decl.(*ast.GenDecl)
		if !ok || gd.Tok != token.IMPORT {
			continue
		}

		filtered := make([]ast.Spec, 0, len(gd.Specs))
		hasGotest := false

		for _, spec := range gd.Specs {
			is, ok := spec.(*ast.ImportSpec)
			if !ok {
				filtered = append(filtered, spec)
				continue
			}
			path := strings.Trim(is.Path.Value, `"`)
			if removable[path] && !used[importName(is)] {
				continue
			}
			if path == gotestImport {
				hasGotest = true
			}
			filtered = append(filtered, spec)
		}

		if !hasGotest {
			filtered = append(filtered, &ast.ImportSpec{
				Path: &ast.BasicLit{
					Kind:  token.STRING,
					Value: fmt.Sprintf("%q", gotestImport),
				},
			})
		}

		gd.Specs = filtered
	}
}

// importName is the name an import is referred to by: its alias, or the
// last element of its path.
func importName(is *ast.ImportSpec) string {
	if is.Name != nil {
		return is.Name.Name
	}
	path := strings.Trim(is.Path.Value, `"`)
	return path[strings.LastIndex(path, "/")+1:]
}

// rewriteSTCalls rewrites every s.T() in body to t.T(). Only a method that
// gained the t parameter may call it; elsewhere s.T() is left for a marker.
func rewriteSTCalls(body *ast.BlockStmt, recvName string) {
	ast.Inspect(body, func(n ast.Node) bool {
		if ident := stCallReceiver(n, recvName); ident != nil {
			ident.Name = "t"
		}
		return true
	})
}

// stCallReceiver returns the receiver identifier of an s.T() call, nil for
// any other node.
func stCallReceiver(n ast.Node, recvName string) *ast.Ident {
	call, ok := n.(*ast.CallExpr)
	if !ok || len(call.Args) != 0 {
		return nil
	}
	sel, ok := call.Fun.(*ast.SelectorExpr)
	if !ok || sel.Sel.Name != "T" {
		return nil
	}
	ident, ok := sel.X.(*ast.Ident)
	if !ok || ident.Name != recvName {
		return nil
	}
	return ident
}

// isRecvRunCall reports whether n is s.Run(...), testify's subtest form,
// which has no gotest equivalent.
func isRecvRunCall(n ast.Node, recvName string) bool {
	call, ok := n.(*ast.CallExpr)
	if !ok {
		return false
	}
	sel, ok := call.Fun.(*ast.SelectorExpr)
	if !ok || sel.Sel.Name != "Run" {
		return false
	}
	ident, ok := sel.X.(*ast.Ident)
	return ok && ident.Name == recvName
}

// tParamKind reports which t fd declares: "gotest" for t *gotest.T,
// "testing" for t *testing.T, "" for none. Both take a gotest assertion; only
// *gotest.T has the T() an s.T() becomes.
func tParamKind(fd *ast.FuncDecl) string {
	if fd.Type.Params == nil {
		return ""
	}
	for _, p := range fd.Type.Params.List {
		if len(p.Names) == 0 || p.Names[0].Name != "t" {
			continue
		}
		star, ok := p.Type.(*ast.StarExpr)
		if !ok {
			continue
		}
		sel, ok := star.X.(*ast.SelectorExpr)
		if !ok || sel.Sel.Name != "T" {
			continue
		}
		if ident, ok := sel.X.(*ast.Ident); ok && (ident.Name == "gotest" || ident.Name == "testing") {
			return ident.Name
		}
	}
	return ""
}

// todoAnnotation marks a line in the migrated output that needs a
// TODO(gotest-migrate) comment inserted above it.
type todoAnnotation struct {
	line int
	msg  string
}

// unmappedAssertionName inspects a call expression in migrated output and
// returns the assertion method name if the call is a testify assertion the
// migrator could not map. Recognized receiver forms: s.Require().X(...),
// s.Assert().X(...), direct s.X(...) for known testify assertion names, and
// package-level assert.X(...) / require.X(...). Without t in scope even a
// mapped s.Require().X(...) stays as it was.
func unconvertedAssertionMsg(call *ast.CallExpr, recvName string, hasT bool) (string, bool) {
	sel, ok := call.Fun.(*ast.SelectorExpr)
	if !ok {
		return "", false
	}
	name := sel.Sel.Name
	gotestName, mapped := assertionMap[name]

	switch x := sel.X.(type) {
	case *ast.CallExpr:
		// s.Require().X(...) or s.Assert().X(...) — mapped names are rewritten
		// where t is in scope.
		if mapped && hasT {
			return "", false
		}
		innerSel, ok := x.Fun.(*ast.SelectorExpr)
		if !ok {
			return "", false
		}
		ident, ok := innerSel.X.(*ast.Ident)
		if !ok || ident.Name != recvName {
			return "", false
		}
		if innerSel.Sel.Name != "Require" && innerSel.Sel.Name != "Assert" {
			return "", false
		}
		if mapped {
			return "assertion " + name + " outside a migrated method — no t is in scope here; pass t in and rewrite as gotest." + gotestName + "(t, ...)", true
		}
		return "unmapped assertion " + name + " — convert manually", true
	case *ast.Ident:
		// assert.X(...) or require.X(...) — mapped names are rewritten.
		if x.Name == "assert" || x.Name == "require" {
			if mapped {
				return "", false
			}
			return "unmapped assertion " + name + " — convert manually", true
		}
		// Direct s.X(...) is never rewritten — the suite.Suite embedding is
		// removed, so these become compile errors. Annotate mapped names with
		// the replacement, unmapped ones generically.
		if x.Name == recvName && isTestifyAssertionName(name) {
			if mapped {
				return "unconverted assertion " + name + " — embedded-suite call; rewrite as gotest." + gotestName + "(t, ...)", true
			}
			return "unmapped assertion " + name + " — convert manually", true
		}
	}
	return "", false
}

// annotateUnconverted parses formatted migrated source and inserts
// `// TODO(gotest-migrate): ...` comments above unconverted testify lifecycle
// hooks, unmapped assertion calls, s.Run and any s.T() left in a method
// without t, so nothing is silently skipped. It operates line-based on the
// already-formatted output (positions are stable at that point) and
// re-formats the result.
func annotateUnconverted(src []byte, suiteNames map[string]bool) ([]byte, error) {
	fset := token.NewFileSet()
	f, err := parser.ParseFile(fset, "", src, parser.ParseComments)
	if err != nil {
		return nil, err
	}

	var anns []todoAnnotation
	seen := map[string]bool{}
	add := func(pos token.Pos, msg string) {
		line := fset.Position(pos).Line
		key := fmt.Sprintf("%d:%s", line, msg)
		if seen[key] {
			return
		}
		seen[key] = true
		anns = append(anns, todoAnnotation{line: line, msg: msg})
	}

	for _, decl := range f.Decls {
		fd, ok := decl.(*ast.FuncDecl)
		if !ok {
			continue
		}
		vars, onSuite := suiteVars(fd, suiteNames)

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
		for _, v := range vars {
			ast.Inspect(fd.Body, func(n ast.Node) bool {
				call, ok := n.(*ast.CallExpr)
				if !ok {
					return true
				}
				if msg, found := unconvertedAssertionMsg(call, v, kind != ""); found {
					add(call.Pos(), msg)
				}
				if isRecvRunCall(call, v) {
					add(call.Pos(), v+".Run has no gotest equivalent — use t.When or t.It")
				}
				if kind != "gotest" && stCallReceiver(call, v) != nil {
					msg := v + ".T() outside a migrated method — no t is in scope here"
					if kind == "testing" {
						msg = v + ".T() where t is a *testing.T — use t"
					}
					add(call.Pos(), msg)
				}
				return true
			})
		}
	}

	return insertMarkers(src, anns)
}

// insertMarkers writes one TODO(gotest-migrate) comment above each annotated
// line and returns the formatted result.
func insertMarkers(src []byte, anns []todoAnnotation) ([]byte, error) {
	if len(anns) == 0 {
		return src, nil
	}

	// Insert comment lines bottom-up so earlier line numbers stay valid.
	sort.Slice(anns, func(i, j int) bool { return anns[i].line > anns[j].line })
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

// indirectEmbeddings lists every struct that embeds one of the file's suites
// instead of suite.Suite. Renaming the base would leave the child behind, so
// such a file is refused whole.
func indirectEmbeddings(f *ast.File, plan MigrationPlan) []todoAnnotation {
	bases := map[string]bool{}
	for i := range plan.Suites {
		bases[plan.Suites[i].OldName] = true
	}
	var anns []todoAnnotation
	for _, decl := range f.Decls {
		gd, ok := decl.(*ast.GenDecl)
		if !ok || gd.Tok != token.TYPE {
			continue
		}
		for _, spec := range gd.Specs {
			ts, ok := spec.(*ast.TypeSpec)
			if !ok {
				continue
			}
			st, ok := ts.Type.(*ast.StructType)
			if !ok || st.Fields == nil {
				continue
			}
			for _, field := range st.Fields.List {
				if len(field.Names) != 0 {
					continue
				}
				base := gotestast.ReceiverTypeName(field.Type)
				if !bases[base] {
					continue
				}
				anns = append(anns, todoAnnotation{
					line: 0, // filled by the caller, which owns the FileSet
					msg:  ts.Name.Name + " embeds " + base + ", not suite.Suite — migrate this file by hand",
				})
				anns[len(anns)-1].line = -int(gd.Pos()) // marker for the caller
			}
		}
	}
	return anns
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

// Options steer a migration run.
type Options struct {
	// DryRun writes nothing; the unified diff of every edit goes to Out.
	DryRun bool
	Out    io.Writer
}

// MigrateFile processes a single file: parse, analyze, transform, format, write back.
func MigrateFile(path string) ([]MigrateResult, error) {
	return migrateFile(path, nil, Options{})
}

// migrateFile migrates path; siblings maps the suites other files of its
// package rename, old name to new.
func migrateFile(path string, siblings map[string]string, opts Options) ([]MigrateResult, error) {
	before, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read %s: %w", path, err)
	}
	results, after, err := migrateSource(path, before, siblings)
	if err != nil || after == nil {
		return results, err
	}
	if opts.DryRun {
		if opts.Out != nil {
			fmt.Fprint(opts.Out, unifiedDiff(path, before, after))
		}
		return results, nil
	}
	if err := os.WriteFile(path, after, 0644); err != nil { //nolint:gosec // G306: not sensitive data
		return nil, fmt.Errorf("write %s: %w", path, err)
	}
	return results, nil
}

// markerPrefix opens every TODO(gotest-migrate) comment.
const markerPrefix = "// TODO(gotest-migrate):"

// countMarkers counts the TODO(gotest-migrate) comments in f; the text in a
// string literal is not one.
func countMarkers(f *ast.File) int {
	n := 0
	for _, cg := range f.Comments {
		for _, c := range cg.List {
			if strings.HasPrefix(c.Text, markerPrefix) {
				n++
			}
		}
	}
	return n
}

// countSourceMarkers is countMarkers for formatted source.
func countSourceMarkers(src []byte) (int, error) {
	f, err := parser.ParseFile(token.NewFileSet(), "", src, parser.ParseComments)
	if err != nil {
		return 0, err
	}
	return countMarkers(f), nil
}

// migrateSource returns the migrated form of src, or nil when there is
// nothing to change. A file without a suite of its own has only its
// references to renamed sibling suites rewritten, and still reports the
// markers an earlier run left in it.
func migrateSource(path string, src []byte, siblings map[string]string) ([]MigrateResult, []byte, error) {
	fset := token.NewFileSet()
	f, err := parser.ParseFile(fset, path, src, parser.ParseComments)
	if err != nil {
		return nil, nil, fmt.Errorf("parse %s: %w", path, err)
	}

	plan := AnalyzeFile(f)
	if len(plan.Suites) == 0 {
		if renameTypeRefs(f, siblings) {
			return migrateRefs(fset, f, path, siblings)
		}
		if n := countMarkers(f); n > 0 {
			return []MigrateResult{{File: path, Markers: n}}, nil, nil
		}
		return nil, nil, nil
	}

	if indirect := indirectEmbeddings(f, plan); len(indirect) > 0 {
		// The file is left as it is, apart from a marker at every child.
		reasons := make([]string, 0, len(indirect))
		for i := range indirect {
			indirect[i].line = fset.Position(token.Pos(-indirect[i].line)).Line
			reasons = append(reasons, indirect[i].msg)
		}
		marked, err := insertMarkers(src, indirect)
		if err != nil {
			return nil, nil, fmt.Errorf("annotate %s: %w", path, err)
		}
		markers, err := countSourceMarkers(marked)
		if err != nil {
			return nil, nil, fmt.Errorf("annotate %s: %w", path, err)
		}
		return []MigrateResult{{File: path, Markers: markers, Refusal: strings.Join(reasons, "; ")}}, marked, nil
	}

	transformFile(f, plan, siblings)

	suiteNames := map[string]bool{}
	for i := range plan.Suites {
		suiteNames[plan.Suites[i].NewName] = true
	}
	for _, nn := range siblings {
		suiteNames[nn] = true
	}
	formatted, markers, err := finish(fset, f, path, suiteNames)
	if err != nil {
		return nil, nil, err
	}

	var results []MigrateResult
	for i := range plan.Suites {
		results = append(results, MigrateResult{
			File:    path,
			OldName: plan.Suites[i].OldName,
			NewName: plan.Suites[i].NewName,
			Markers: markers,
		})
		markers = 0 // the file's markers are counted once, on its first suite
	}
	return results, formatted, nil
}

// migrateRefs finishes a file whose only change is its references to
// renamed sibling suites; a helper taking one is annotated like a suite's.
func migrateRefs(fset *token.FileSet, f *ast.File, path string, siblings map[string]string) ([]MigrateResult, []byte, error) {
	suiteNames := map[string]bool{}
	for _, nn := range siblings {
		suiteNames[nn] = true
	}
	formatted, markers, err := finish(fset, f, path, suiteNames)
	if err != nil {
		return nil, nil, err
	}
	return []MigrateResult{{File: path, Markers: markers, RefsOnly: true}}, formatted, nil
}

// finish formats the transformed f, annotates anything the migrator could
// not convert with TODO(gotest-migrate) markers so nothing is silently
// skipped, and counts the markers the file is left with.
func finish(fset *token.FileSet, f *ast.File, path string, suiteNames map[string]bool) ([]byte, int, error) {
	var buf bytes.Buffer
	if err := format.Node(&buf, fset, f); err != nil {
		return nil, 0, fmt.Errorf("format %s: %w", path, err)
	}
	formatted, err := format.Source(buf.Bytes())
	if err != nil {
		return nil, 0, fmt.Errorf("gofmt %s: %w", path, err)
	}
	formatted, err = annotateUnconverted(formatted, suiteNames)
	if err != nil {
		return nil, 0, fmt.Errorf("annotate %s: %w", path, err)
	}
	markers, err := countSourceMarkers(formatted)
	if err != nil {
		return nil, 0, fmt.Errorf("annotate %s: %w", path, err)
	}
	return formatted, markers, nil
}

// packageRenames maps each package (directory and package clause) to the
// suites its files rename, old name to new. A refused file renames nothing.
func packageRenames(paths []string) (map[string]map[string]string, error) {
	renames := map[string]map[string]string{}
	for _, path := range paths {
		f, err := parser.ParseFile(token.NewFileSet(), path, nil, parser.SkipObjectResolution)
		if err != nil {
			return nil, fmt.Errorf("parse %s: %w", path, err)
		}
		plan := AnalyzeFile(f)
		if len(plan.Suites) == 0 || len(indirectEmbeddings(f, plan)) > 0 {
			continue
		}
		key := packageKey(path, f)
		if renames[key] == nil {
			renames[key] = map[string]string{}
		}
		for i := range plan.Suites {
			renames[key][plan.Suites[i].OldName] = plan.Suites[i].NewName
		}
	}
	return renames, nil
}

func packageKey(path string, f *ast.File) string {
	return filepath.Dir(path) + "\x00" + f.Name.Name
}

// MigratePackages walks directories matching patterns and migrates test files.
func MigratePackages(patterns []string, opts Options) ([]MigrateResult, error) {
	var allResults []MigrateResult
	var paths []string

	for _, pattern := range patterns {
		dir := pattern
		recursive := false
		if strings.HasSuffix(dir, "/...") {
			dir = strings.TrimSuffix(dir, "/...")
			recursive = true
		}
		if dir == "" || dir == "." {
			dir = "."
		}

		err := filepath.WalkDir(dir, func(path string, d os.DirEntry, err error) error {
			if err != nil {
				if path == dir {
					return fmt.Errorf("pattern %s: %w", pattern, err)
				}
				return nil
			}
			if d.IsDir() {
				if !recursive && path != dir {
					return filepath.SkipDir
				}
				return nil
			}
			if !strings.HasSuffix(d.Name(), "_test.go") {
				return nil
			}
			paths = append(paths, path)
			return nil
		})
		if err != nil {
			return nil, err
		}
	}

	// A file may refer to a suite another file of its package renames.
	renames, err := packageRenames(paths)
	if err != nil {
		return nil, err
	}
	for _, path := range paths {
		f, err := parser.ParseFile(token.NewFileSet(), path, nil, parser.PackageClauseOnly)
		if err != nil {
			return allResults, fmt.Errorf("migrate %s: %w", path, err)
		}
		results, err := migrateFile(path, renames[packageKey(path, f)], opts)
		if err != nil {
			return allResults, fmt.Errorf("migrate %s: %w", path, err)
		}
		allResults = append(allResults, results...)
	}
	return allResults, nil
}
