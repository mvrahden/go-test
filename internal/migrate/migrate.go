package migrate

import (
	"bytes"
	"fmt"
	"go/ast"
	"go/format"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"strings"
)

const gotestImport = "github.com/mvrahden/go-test/pkg/gotest"

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

// MigrateResult describes what was migrated in a file.
type MigrateResult struct {
	File    string
	OldName string
	NewName string
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
	switch t := field.Type.(type) {
	case *ast.StarExpr:
		if ident, ok := t.X.(*ast.Ident); ok {
			typeName = ident.Name
		}
	case *ast.Ident:
		typeName = t.Name
	}
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

	// assertionMap maps testify assertion method names to gotest function names
	assertionMap := map[string]string{
		"Equal":          "Equal",
		"NotEqual":       "NotEqual",
		"NoError":        "NoError",
		"Error":          "Error",
		"ErrorIs":        "ErrorIs",
		"True":           "True",
		"False":          "False",
		"Nil":            "Empty",
		"NotNil":         "NotEmpty",
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
			if newName, ok := lifecycleRenames[d.Name.Name]; ok {
				d.Name.Name = newName
				addGotestTParam(d)
			}

			// Test methods: add t parameter
			if strings.HasPrefix(d.Name.Name, "Test") {
				addGotestTParam(d)
			}

			// Rewrite assertions in the function body
			if d.Body != nil {
				rewriteAssertions(d.Body, recvVarName, assertionMap)
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

// rewriteSTandalone rewrites s.T() when used as an argument (not in assertion chains)
// to t.T(). This is done as a post-processing step on the formatted source.
func rewriteSTandalone(src string, recvName string) string {
	// Replace s.T() with t.T() — but only where it hasn't already been rewritten
	// The assertion rewriting handles assertion contexts. Here we handle when
	// s.T() is passed as an argument to non-assertion functions.
	old := recvName + ".T()"
	new := "t.T()"
	return strings.ReplaceAll(src, old, new)
}

// MigrateFile processes a single file: parse, analyze, transform, format, write back.
func MigrateFile(path string) ([]MigrateResult, error) {
	fset := token.NewFileSet()
	f, err := parser.ParseFile(fset, path, nil, parser.ParseComments)
	if err != nil {
		return nil, fmt.Errorf("parse %s: %w", path, err)
	}

	plan := AnalyzeFile(f)
	if len(plan.Suites) == 0 {
		return nil, nil
	}

	TransformFile(fset, f, plan)

	var buf bytes.Buffer
	if err := format.Node(&buf, fset, f); err != nil {
		return nil, fmt.Errorf("format %s: %w", path, err)
	}

	// Post-process: rewrite remaining s.T() to t.T()
	src := buf.String()
	for i := range plan.Suites {
		if plan.Suites[i].ReceiverName != "" {
			src = rewriteSTandalone(src, plan.Suites[i].ReceiverName)
		}
	}

	formatted, err := format.Source([]byte(src))
	if err != nil {
		return nil, fmt.Errorf("gofmt %s: %w", path, err)
	}

	if err := os.WriteFile(path, formatted, 0644); err != nil { //nolint:gosec // G306: not sensitive data
		return nil, fmt.Errorf("write %s: %w", path, err)
	}

	var results []MigrateResult
	for i := range plan.Suites {
		results = append(results, MigrateResult{
			File:    path,
			OldName: plan.Suites[i].OldName,
			NewName: plan.Suites[i].NewName,
		})
	}
	return results, nil
}

// MigratePackages walks directories matching patterns and migrates test files.
func MigratePackages(patterns []string) ([]MigrateResult, error) {
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

			results, err := MigrateFile(path)
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
