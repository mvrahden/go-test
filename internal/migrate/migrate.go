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
	// Markers counts the TODO(gotest-migrate) lines the file was left with.
	Markers int
	// Refusal names why the file was not migrated at all; markers stand
	// where the reason was found.
	Refusal string
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
			if d.Recv == nil || len(d.Recv.List) == 0 {
				continue
			}
			recvTypeName, recvVarName := extractReceiverInfo(d.Recv.List[0])
			if !suiteOldNames[recvTypeName] {
				continue
			}

			// Rename receiver type
			renameReceiverType(d.Recv.List[0], oldToNew[recvTypeName])

			// Rename lifecycle methods and add t parameter
			gainedT := false
			if newName, ok := lifecycleRenames[d.Name.Name]; ok {
				d.Name.Name = newName
				addGotestTParam(d)
				gainedT = true
			}

			// Test methods: add t parameter
			if strings.HasPrefix(d.Name.Name, "Test") {
				addGotestTParam(d)
				gainedT = true
			}

			// Rewrite assertions in the function body
			if d.Body != nil {
				rewriteAssertions(d.Body, recvVarName, assertionMap)
				if gainedT {
					rewriteSTCalls(d.Body, recvVarName)
				}
			}
		}
	}

	// 3. Rewrite imports
	rewriteImports(f)
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

// rewriteImports removes testify imports and adds gotest import.
func rewriteImports(f *ast.File) {
	testifyPaths := map[string]bool{
		"github.com/stretchr/testify/suite":   true,
		"github.com/stretchr/testify/assert":  true,
		"github.com/stretchr/testify/require": true,
	}

	// Check if "testing" is still referenced after removing runner funcs
	testingStillUsed := false
	ast.Inspect(f, func(n ast.Node) bool {
		sel, ok := n.(*ast.SelectorExpr)
		if !ok {
			return true
		}
		if ident, ok := sel.X.(*ast.Ident); ok && ident.Name == "testing" {
			testingStillUsed = true
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
			if testifyPaths[path] {
				continue // remove testify imports
			}
			if path == "testing" && !testingStillUsed {
				continue // remove testing if no longer needed
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

// hasGotestTParam reports whether fd declares a t *gotest.T parameter.
func hasGotestTParam(fd *ast.FuncDecl) bool {
	if fd.Type.Params == nil {
		return false
	}
	for _, p := range fd.Type.Params.List {
		star, ok := p.Type.(*ast.StarExpr)
		if !ok {
			continue
		}
		sel, ok := star.X.(*ast.SelectorExpr)
		if !ok || sel.Sel.Name != "T" {
			continue
		}
		if ident, ok := sel.X.(*ast.Ident); ok && ident.Name == "gotest" {
			return true
		}
	}
	return false
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
// package-level assert.X(...) / require.X(...).
func unconvertedAssertionMsg(call *ast.CallExpr, recvName string) (string, bool) {
	sel, ok := call.Fun.(*ast.SelectorExpr)
	if !ok {
		return "", false
	}
	name := sel.Sel.Name
	gotestName, mapped := assertionMap[name]

	switch x := sel.X.(type) {
	case *ast.CallExpr:
		// s.Require().X(...) or s.Assert().X(...) — mapped names are rewritten.
		if mapped {
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
		if innerSel.Sel.Name == "Require" || innerSel.Sel.Name == "Assert" {
			return "unmapped assertion " + name + " — convert manually", true
		}
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
// already-formatted output (positions are stable at that point), re-formats
// the result and returns how many markers it placed.
func annotateUnconverted(src []byte, suiteNames map[string]bool) ([]byte, int, error) {
	fset := token.NewFileSet()
	f, err := parser.ParseFile(fset, "", src, parser.ParseComments)
	if err != nil {
		return nil, 0, err
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
		if !ok || fd.Recv == nil || len(fd.Recv.List) == 0 {
			continue
		}
		recvTypeName, recvVarName := extractReceiverInfo(fd.Recv.List[0])
		if !suiteNames[recvTypeName] {
			continue
		}

		if unsupportedHooks[fd.Name.Name] {
			pos := fd.Pos()
			if fd.Doc != nil {
				pos = fd.Doc.Pos()
			}
			add(pos, "unsupported testify hook "+fd.Name.Name+" — convert manually")
		}

		if fd.Body == nil {
			continue
		}
		hasT := hasGotestTParam(fd)
		ast.Inspect(fd.Body, func(n ast.Node) bool {
			call, ok := n.(*ast.CallExpr)
			if !ok {
				return true
			}
			if msg, found := unconvertedAssertionMsg(call, recvVarName); found {
				add(call.Pos(), msg)
			}
			if isRecvRunCall(call, recvVarName) {
				add(call.Pos(), recvVarName+".Run has no gotest equivalent — use t.When or t.It")
			}
			if !hasT && stCallReceiver(call, recvVarName) != nil {
				add(call.Pos(), recvVarName+".T() outside a migrated method — no t is in scope here")
			}
			return true
		})
	}

	return insertMarkers(src, anns)
}

// insertMarkers writes one TODO(gotest-migrate) comment above each annotated
// line and returns the formatted result with the number of markers added.
func insertMarkers(src []byte, anns []todoAnnotation) ([]byte, int, error) {
	if len(anns) == 0 {
		return src, 0, nil
	}

	// Insert comment lines bottom-up so earlier line numbers stay valid.
	sort.Slice(anns, func(i, j int) bool { return anns[i].line > anns[j].line })
	lines := strings.Split(string(src), "\n")
	added := 0
	for _, a := range anns {
		idx := a.line - 1
		if idx < 0 || idx >= len(lines) {
			continue
		}
		comment := "// TODO(gotest-migrate): " + a.msg
		if idx > 0 && strings.Contains(lines[idx-1], comment) {
			continue // already annotated (idempotency)
		}
		indented := leadingWhitespace(lines[idx]) + comment
		lines = append(lines[:idx], append([]string{indented}, lines[idx:]...)...)
		added++
	}
	out, err := format.Source([]byte(strings.Join(lines, "\n")))
	return out, added, err
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
	return migrateFile(path, Options{})
}

func migrateFile(path string, opts Options) ([]MigrateResult, error) {
	before, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read %s: %w", path, err)
	}
	results, after, err := migrateSource(path, before)
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

// migrateSource returns the migrated form of src, or nil when the file holds
// no testify suite.
func migrateSource(path string, src []byte) ([]MigrateResult, []byte, error) {
	fset := token.NewFileSet()
	f, err := parser.ParseFile(fset, path, src, parser.ParseComments)
	if err != nil {
		return nil, nil, fmt.Errorf("parse %s: %w", path, err)
	}

	plan := AnalyzeFile(f)
	if len(plan.Suites) == 0 {
		return nil, nil, nil
	}

	if indirect := indirectEmbeddings(f, plan); len(indirect) > 0 {
		// The file is left as it is, apart from a marker at every child.
		reasons := make([]string, 0, len(indirect))
		for i := range indirect {
			indirect[i].line = fset.Position(token.Pos(-indirect[i].line)).Line
			reasons = append(reasons, indirect[i].msg)
		}
		marked, added, err := insertMarkers(src, indirect)
		if err != nil {
			return nil, nil, fmt.Errorf("annotate %s: %w", path, err)
		}
		return []MigrateResult{{File: path, Markers: added, Refusal: strings.Join(reasons, "; ")}}, marked, nil
	}

	TransformFile(fset, f, plan)

	var buf bytes.Buffer
	if err := format.Node(&buf, fset, f); err != nil {
		return nil, nil, fmt.Errorf("format %s: %w", path, err)
	}
	formatted, err := format.Source(buf.Bytes())
	if err != nil {
		return nil, nil, fmt.Errorf("gofmt %s: %w", path, err)
	}

	// Annotate anything the migrator could not convert with TODO(gotest-migrate)
	// markers so nothing is silently skipped.
	suiteNames := map[string]bool{}
	for i := range plan.Suites {
		suiteNames[plan.Suites[i].NewName] = true
	}
	formatted, markers, err := annotateUnconverted(formatted, suiteNames)
	if err != nil {
		return nil, nil, fmt.Errorf("annotate %s: %w", path, err)
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

// MigratePackages walks directories matching patterns and migrates test files.
func MigratePackages(patterns []string, opts Options) ([]MigrateResult, error) {
	var allResults []MigrateResult

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

			results, err := migrateFile(path, opts)
			if err != nil {
				return fmt.Errorf("migrate %s: %w", path, err)
			}
			allResults = append(allResults, results...)
			return nil
		})
		if err != nil {
			return allResults, err
		}
	}

	return allResults, nil
}
