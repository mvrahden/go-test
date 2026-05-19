package lint

import (
	"fmt"
	"go/ast"
	"go/token"
	"path/filepath"
	"slices"
	"strings"
	"unicode"

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

	return nil, nil
}

func report(pass *analysis.Pass, rule Rule, pos token.Pos, format string, args ...any) {
	pass.Report(analysis.Diagnostic{
		Pos:      pos,
		Category: string(rule),
		Message:  fmt.Sprintf(format, args...),
	})
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
		if strings.HasPrefix(name, "ƒƒ_") && strings.HasSuffix(name, "_test.go") {
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
	return strings.HasPrefix(filepath.Base(pass.Fset.Position(pos).Filename), "ƒƒ_")
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
	switch t := recv.List[0].Type.(type) {
	case *ast.StarExpr:
		if ident, ok := t.X.(*ast.Ident); ok {
			return ident.Name
		}
	case *ast.Ident:
		return t.Name
	case *ast.IndexExpr:
		if ident, ok := t.X.(*ast.Ident); ok {
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
