package lint

import (
	"fmt"
	"go/ast"
	"go/token"
	"path/filepath"
	"slices"
	"strings"
	"unicode"

	"github.com/mvrahden/go-test/about"
	"golang.org/x/tools/go/analysis"
	"golang.org/x/tools/go/analysis/passes/inspect"
	"golang.org/x/tools/go/ast/inspector"
)

// Rule identifies a specific lint check.
type Rule string

const (
	Focus         Rule = "focus"
	Receiver      Rule = "receiver"
	LifecycleTypo Rule = "lifecycle-typo"
	LifecyclePair Rule = "lifecycle-pair"
	GeneratedFile Rule = "generated-file"
	StdlibTest    Rule = "stdlib-test"
	Testify       Rule = "testify"
	PollScope     Rule = "poll-scope"
)

// SkippableRules is the set of rules that support opt-out via skip flags.
var SkippableRules = map[Rule]bool{
	StdlibTest: true,
	Testify:    true,
}

var cfg struct {
	skipStdlibTest bool
	skipTestify    bool
}

var Analyzer = &analysis.Analyzer{
	Name:     "gotestlint",
	Doc:      "checks for common mistakes in gotest test suites",
	Requires: []*analysis.Analyzer{inspect.Analyzer},
	Run:      run,
}

func init() {
	Analyzer.Flags.BoolVar(&cfg.skipStdlibTest, "skip-stdlib-test", false, "disable stdlib test function detection")
	Analyzer.Flags.BoolVar(&cfg.skipTestify, "skip-testify", false, "disable testify import detection")
}

var lifecycleHooks = []string{"BeforeAll", "AfterAll", "BeforeEach", "AfterEach"}

func run(pass *analysis.Pass) (any, error) {
	insp := pass.ResultOf[inspect.Analyzer].(*inspector.Inspector)

	suites := discoverSuites(insp)
	if len(suites) > 0 {
		checkFocusPrefixes(pass, suites)
		checkMethods(pass, insp, suites)
		checkLifecyclePairs(pass, suites)
	}

	checkOrphanedFiles(pass)
	checkStdlibTests(pass, insp)
	checkTestifyImports(pass)
	checkPollScope(pass, insp)

	return nil, nil
}

func report(pass *analysis.Pass, rule Rule, pos token.Pos, format string, args ...any) {
	if isSuppressed(pass, pos, rule) {
		return
	}
	pass.Report(analysis.Diagnostic{
		Pos:      pos,
		Category: string(rule),
		Message:  fmt.Sprintf(format, args...),
	})
}

func isSuppressed(pass *analysis.Pass, pos token.Pos, rule Rule) bool {
	position := pass.Fset.Position(pos)
	for _, file := range pass.Files {
		if pass.Fset.Position(file.Pos()).Filename != position.Filename {
			continue
		}
		pkgLine := pass.Fset.Position(file.Package).Line
		for _, cg := range file.Comments {
			for _, c := range cg.List {
				rules, ok := parseNolint(c.Text)
				if !ok {
					continue
				}
				if rules != nil && !rules[rule] {
					continue
				}
				cLine := pass.Fset.Position(c.Pos()).Line
				if cLine == pkgLine {
					return true
				}
				if cLine == position.Line {
					return true
				}
			}
		}
		return false
	}
	return false
}

func docSuppressed(doc *ast.CommentGroup, rule Rule) bool {
	if doc == nil {
		return false
	}
	for _, c := range doc.List {
		rules, ok := parseNolint(c.Text)
		if !ok {
			continue
		}
		if rules == nil || rules[rule] {
			return true
		}
	}
	return false
}

func parseNolint(text string) (rules map[Rule]bool, ok bool) {
	if !strings.HasPrefix(text, "//nolint") {
		return nil, false
	}
	rest := text[len("//nolint"):]
	if rest == "" {
		return nil, true
	}
	if rest[0] != ':' {
		return nil, false
	}
	rest = rest[1:]
	if idx := strings.Index(rest, " //"); idx >= 0 {
		rest = rest[:idx]
	}
	rest = strings.TrimSpace(rest)
	if rest == "" {
		return nil, true
	}
	rules = make(map[Rule]bool)
	for _, r := range strings.Split(rest, ",") {
		r = strings.TrimSpace(r)
		if r != "" {
			rules[Rule(r)] = true
		}
	}
	if len(rules) == 0 {
		return nil, true
	}
	return rules, true
}

type suiteInfo struct {
	name    string
	pos     token.Pos
	methods map[string]token.Pos
}

func discoverSuites(insp *inspector.Inspector) map[string]*suiteInfo {
	suites := make(map[string]*suiteInfo)

	insp.Preorder([]ast.Node{(*ast.GenDecl)(nil)}, func(n ast.Node) {
		gd := n.(*ast.GenDecl)
		if gd.Tok != token.TYPE {
			return
		}
		for _, spec := range gd.Specs {
			ts, ok := spec.(*ast.TypeSpec)
			if !ok {
				continue
			}
			name := ts.Name.Name
			stripped := strings.TrimPrefix(strings.TrimPrefix(name, "F_"), "X_")
			if strings.HasSuffix(stripped, "TestSuite") {
				suites[name] = &suiteInfo{
					name:    name,
					pos:     ts.Pos(),
					methods: make(map[string]token.Pos),
				}
			}
		}
	})

	return suites
}

// X_ (skip) prefixes are intentionally not flagged: a skipped test is visibly
// absent from results, whereas a focused test silently hides all other tests
// behind a green CI run.
func checkFocusPrefixes(pass *analysis.Pass, suites map[string]*suiteInfo) {
	for name, s := range suites {
		if strings.HasPrefix(name, "F_") {
			report(pass, Focus, s.pos, "focused suite %s should not be committed", name)
		}
	}
}

func checkMethods(pass *analysis.Pass, insp *inspector.Inspector, suites map[string]*suiteInfo) {
	insp.Preorder([]ast.Node{(*ast.FuncDecl)(nil)}, func(n ast.Node) {
		fd := n.(*ast.FuncDecl)
		if fd.Recv == nil || len(fd.Recv.List) == 0 {
			return
		}

		recvName := receiverTypeName(fd.Recv)
		suite, exists := suites[recvName]
		if !exists {
			return
		}

		methodName := fd.Name.Name
		suite.methods[methodName] = fd.Pos()

		if !isPointerReceiver(fd.Recv) {
			report(pass, Receiver, fd.Pos(), "suite method %s.%s should use a pointer receiver", recvName, methodName)
		}

		stripped := strings.TrimPrefix(strings.TrimPrefix(methodName, "F_"), "X_")
		if strings.HasPrefix(stripped, "Test") {
			if strings.HasPrefix(methodName, "F_") {
				report(pass, Focus, fd.Pos(), "focused method %s.%s should not be committed", recvName, methodName)
			}
			return
		}

		if isLifecycleHook(stripped) {
			return
		}

		for _, hook := range lifecycleHooks {
			if levenshtein(stripped, hook) <= 2 {
				report(pass, LifecycleTypo, fd.Pos(), "method %s on suite %s is similar to lifecycle hook %s", methodName, recvName, hook)
				return
			}
		}
	})
}

// Only the All pair is checked: BeforeAll holds shared resources for the
// entire suite lifetime, so a missing AfterAll is a likely leak.  BeforeEach
// resources are scoped to a single test and cleaned up with the test.
func checkLifecyclePairs(pass *analysis.Pass, suites map[string]*suiteInfo) {
	for _, s := range suites {
		_, hasBeforeAll := s.methods["BeforeAll"]
		_, hasAfterAll := s.methods["AfterAll"]
		if hasBeforeAll && !hasAfterAll {
			report(pass, LifecyclePair, s.pos, "suite %s has BeforeAll but no AfterAll — resources may leak", s.name)
		}
	}
}

func checkOrphanedFiles(pass *analysis.Pass) {
	for _, file := range pass.Files {
		name := filepath.Base(pass.Fset.File(file.Pos()).Name())
		if about.PSuiteRegex.MatchString(name) {
			report(pass, GeneratedFile, file.Pos(), "generated file %s should not be checked into version control", name)
		}
	}
}

func checkStdlibTests(pass *analysis.Pass, insp *inspector.Inspector) {
	if cfg.skipStdlibTest {
		return
	}

	insp.Preorder([]ast.Node{(*ast.FuncDecl)(nil)}, func(n ast.Node) {
		fd := n.(*ast.FuncDecl)
		if fd.Recv != nil {
			return
		}
		name := fd.Name.Name
		if !strings.HasPrefix(name, "Test") {
			return
		}
		if len(name) > 4 && unicode.IsLower(rune(name[4])) {
			return
		}
		if isGeneratedFile(pass, fd.Pos()) {
			return
		}
		if fd.Type.Params == nil || len(fd.Type.Params.List) != 1 {
			return
		}
		if !isTestingT(fd.Type.Params.List[0].Type) {
			return
		}
		if docSuppressed(fd.Doc, StdlibTest) {
			return
		}
		report(pass, StdlibTest, fd.Pos(), "stdlib test %s — consider using a gotest suite", name)
	})
}

func checkTestifyImports(pass *analysis.Pass) {
	if cfg.skipTestify {
		return
	}

	for _, file := range pass.Files {
		if isGeneratedFile(pass, file.Pos()) {
			continue
		}
		for _, imp := range file.Imports {
			path := strings.Trim(imp.Path.Value, `"`)
			if strings.HasPrefix(path, "github.com/stretchr/testify/") {
				report(pass, Testify, imp.Pos(), "testify import %s — consider migrating to gotest", path)
			}
		}
	}
}

func isGeneratedFile(pass *analysis.Pass, pos token.Pos) bool {
	return about.PSuiteRegex.MatchString(filepath.Base(pass.Fset.Position(pos).Filename))
}

func isTestingT(expr ast.Expr) bool {
	star, ok := expr.(*ast.StarExpr)
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

func receiverTypeName(recv *ast.FieldList) string {
	if recv == nil || len(recv.List) == 0 {
		return ""
	}
	t := recv.List[0].Type
	if star, ok := t.(*ast.StarExpr); ok {
		t = star.X
	}
	switch x := t.(type) {
	case *ast.Ident:
		return x.Name
	case *ast.IndexExpr:
		if ident, ok := x.X.(*ast.Ident); ok {
			return ident.Name
		}
	case *ast.IndexListExpr:
		if ident, ok := x.X.(*ast.Ident); ok {
			return ident.Name
		}
	}
	return ""
}

func isPointerReceiver(recv *ast.FieldList) bool {
	if recv == nil || len(recv.List) == 0 {
		return false
	}
	_, ok := recv.List[0].Type.(*ast.StarExpr)
	return ok
}

func isLifecycleHook(name string) bool {
	return slices.Contains(lifecycleHooks, name)
}

// --- poll-scope check ---

var pollScopeAssertionFuncs = map[string]bool{
	"Consistently": true, "Contains": true, "ElementsMatch": true,
	"Empty": true, "Equal": true, "Error": true,
	"ErrorAs": true, "ErrorContains": true, "ErrorIs": true,
	"Eventually": true, "Fail": true, "False": true,
	"Greater": true, "GreaterOrEqual": true, "InDelta": true,
	"JSONEq": true, "Len": true, "Less": true,
	"LessOrEqual": true, "MatchSnapshot": true, "NoError": true,
	"NotContains": true, "NotEmpty": true, "NotEqual": true,
	"NotZero": true, "Panics": true, "Regexp": true,
	"Subset": true, "TimeIsNow": true, "TimeWithin": true,
	"True": true, "Zero": true,
}

var pollScopeMethodNames = map[string]bool{
	"Errorf":  true,
	"Fatal":   true,
	"Fatalf":  true,
	"FailNow": true,
}

func checkPollScope(pass *analysis.Pass, insp *inspector.Inspector) {
	insp.Preorder([]ast.Node{(*ast.CallExpr)(nil)}, func(n ast.Node) {
		call := n.(*ast.CallExpr)

		fnName := pollingFuncName(call)
		if fnName == "" {
			return
		}

		pollParam, funcLit := extractPollCallback(call)
		if funcLit == nil {
			return
		}

		ast.Inspect(funcLit.Body, func(n ast.Node) bool {
			innerCall, ok := n.(*ast.CallExpr)
			if !ok {
				return true
			}

			// Case 1: gotest assertion with wrong first arg — gotest.Equal(t, ...) or Equal(t, ...)
			if name := resolveAssertionName(innerCall.Fun); name != "" && len(innerCall.Args) > 0 {
				if ident, ok := innerCall.Args[0].(*ast.Ident); ok && ident.Name != pollParam {
					report(pass, PollScope, ident.Pos(),
						"use %s instead of %s in poll callback passed to %s",
						pollParam, ident.Name, fnName)
				}
				return true
			}

			// Case 2: direct method call — t.Errorf(...), t.Fatal(...)
			sel, ok := innerCall.Fun.(*ast.SelectorExpr)
			if !ok {
				return true
			}
			ident, ok := sel.X.(*ast.Ident)
			if !ok {
				return true
			}
			if pollScopeMethodNames[sel.Sel.Name] && ident.Name != pollParam {
				report(pass, PollScope, ident.Pos(),
					"%s.%s in poll callback bypasses assertion recording — use %s",
					ident.Name, sel.Sel.Name, pollParam)
			}
			return true
		})
	})
}

func pollingFuncName(call *ast.CallExpr) string {
	switch fn := call.Fun.(type) {
	case *ast.SelectorExpr:
		if fn.Sel.Name == "Eventually" || fn.Sel.Name == "Consistently" {
			return fn.Sel.Name
		}
	case *ast.Ident:
		if fn.Name == "Eventually" || fn.Name == "Consistently" {
			return fn.Name
		}
	}
	return ""
}

func extractPollCallback(call *ast.CallExpr) (string, *ast.FuncLit) {
	if len(call.Args) == 0 {
		return "", nil
	}
	lastArg := call.Args[len(call.Args)-1]
	funcLit, ok := lastArg.(*ast.FuncLit)
	if !ok {
		return "", nil
	}
	if funcLit.Type.Params == nil || len(funcLit.Type.Params.List) != 1 {
		return "", nil
	}
	param := funcLit.Type.Params.List[0]
	if !isStarR(param.Type) {
		return "", nil
	}
	if len(param.Names) == 0 {
		return "", nil
	}
	return param.Names[0].Name, funcLit
}

func isStarR(expr ast.Expr) bool {
	star, ok := expr.(*ast.StarExpr)
	if !ok {
		return false
	}
	switch x := star.X.(type) {
	case *ast.Ident:
		return x.Name == "R"
	case *ast.SelectorExpr:
		return x.Sel.Name == "R"
	}
	return false
}

func resolveAssertionName(expr ast.Expr) string {
	switch fn := expr.(type) {
	case *ast.SelectorExpr:
		if pollScopeAssertionFuncs[fn.Sel.Name] {
			return fn.Sel.Name
		}
	case *ast.Ident:
		if pollScopeAssertionFuncs[fn.Name] {
			return fn.Name
		}
	case *ast.IndexExpr:
		return resolveAssertionName(fn.X)
	case *ast.IndexListExpr:
		return resolveAssertionName(fn.X)
	}
	return ""
}

func levenshtein(a, b string) int {
	la, lb := len(a), len(b)
	if la == 0 {
		return lb
	}
	if lb == 0 {
		return la
	}

	prev := make([]int, lb+1)
	curr := make([]int, lb+1)
	for j := 0; j <= lb; j++ {
		prev[j] = j
	}

	for i := 1; i <= la; i++ {
		curr[0] = i
		for j := 1; j <= lb; j++ {
			cost := 1
			if a[i-1] == b[j-1] {
				cost = 0
			}
			curr[j] = min(curr[j-1]+1, min(prev[j]+1, prev[j-1]+cost))
		}
		prev, curr = curr, prev
	}

	return prev[lb]
}
