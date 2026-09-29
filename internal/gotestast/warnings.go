package gotestast

import (
	"fmt"
	"go/ast"
	"go/token"
	"go/types"
	"slices"
	"strings"

	"github.com/mvrahden/go-test/internal/protocol"
	"golang.org/x/tools/go/packages"
)

// LifecycleHooks names the hooks a suite may declare.
var LifecycleHooks = []string{"BeforeAll", "AfterAll", "BeforeEach", "AfterEach"}

// Lint rules that report the same findings as HarnessWarnings.
const (
	RuleLifecycleTypo = "lifecycle-typo"
	RuleXLifecycle    = "x-lifecycle"
)

// Warning is a finding that does not stop generation: a declaration that
// reads like part of a suite's harness and never runs as one.
type Warning struct {
	Pos token.Pos
	// Rule is the lint rule reporting the same finding, empty when none does.
	Rule string
	Msg  string
}

// MisspelledHook returns the lifecycle hook a method name is at most two
// edits away from. A hook's own name is not a misspelling of it.
func MisspelledHook(name string) (hook string, ok bool) {
	if slices.Contains(LifecycleHooks, name) {
		return "", false
	}
	for _, h := range LifecycleHooks {
		if levenshtein(name, h) <= 2 {
			return h, true
		}
	}
	return "", false
}

// HarnessWarnings reports the methods the generator passes over in silence:
// a lifecycle hook that is misspelled or carries X_, and a Benchmark or Fuzz
// method on a type that is no test suite.
func HarnessWarnings(pkg *packages.Package, suites TestSuiteSpecSet) []Warning {
	if pkg == nil || pkg.TypesInfo == nil {
		return nil
	}
	suiteTypes := map[string]bool{}
	for _, s := range suites {
		suiteTypes[s.ts.Name.Name] = true
		if s.underlyingTypeName != "" {
			suiteTypes[s.underlyingTypeName] = true
		}
	}

	var out []Warning
	for _, file := range pkg.Syntax {
		for _, d := range file.Decls {
			decl, ok := d.(*ast.FuncDecl)
			if !ok || decl.Recv == nil || !decl.Name.IsExported() {
				continue
			}
			recv := receiverTypeName(decl)
			if recv == "" {
				continue
			}
			name := decl.Name.Name
			if suiteTypes[recv] {
				if w, ok := hookWarning(recv, name); ok {
					w.Pos = decl.Name.Pos()
					out = append(out, w)
				}
				continue
			}
			if IS_TEST_SUITE.MatchString(recv) {
				continue
			}
			sig, ok := pkg.TypesInfo.TypeOf(decl.Name).(*types.Signature)
			if !ok {
				continue
			}
			var param, kind string
			switch usesStdlib, isB := detectParamB(sig); {
			case IS_BENCHMARK.MatchString(name) && isB && !usesStdlib:
				param, kind = "*gotest.B", "benchmark"
			case IS_FUZZ.MatchString(name) && detectParamF(sig):
				param, kind = "*gotest.F", "fuzz target"
			default:
				continue
			}
			out = append(out, Warning{
				Pos: decl.Name.Pos(),
				Msg: fmt.Sprintf("%s.%s takes %s, but %s is not a test suite (its name must end in TestSuite): the %s never runs", recv, name, param, recv, kind),
			})
		}
	}
	return out
}

func hookWarning(recv, name string) (Warning, bool) {
	stripped := strings.TrimPrefix(strings.TrimPrefix(name, protocol.PrefixFocused), protocol.PrefixExcluded)
	if slices.Contains(LifecycleHooks, stripped) {
		if !strings.HasPrefix(name, protocol.PrefixExcluded) {
			return Warning{}, false
		}
		return Warning{Rule: RuleXLifecycle, Msg: ExcludedHookMessage(recv, name)}, true
	}
	if strings.HasPrefix(stripped, "Test") {
		return Warning{}, false
	}
	if hook, ok := MisspelledHook(stripped); ok {
		return Warning{Rule: RuleLifecycleTypo, Msg: MisspelledHookMessage(recv, name, hook)}, true
	}
	return Warning{}, false
}

// MisspelledHookMessage and ExcludedHookMessage word the two findings lint
// and discover share, so one finding reads the same wherever it is reported.
func MisspelledHookMessage(recv, name, hook string) string {
	return fmt.Sprintf("method %s on suite %s is similar to lifecycle hook %s", name, recv, hook)
}

func ExcludedHookMessage(recv, name string) string {
	return fmt.Sprintf("X_ prefix on lifecycle hook %s.%s has no effect — remove the prefix or the method", recv, name)
}

// receiverTypeName returns the name of the type a method is declared on,
// through a pointer and through type arguments.
func receiverTypeName(decl *ast.FuncDecl) string {
	if len(decl.Recv.List) != 1 {
		return ""
	}
	expr := decl.Recv.List[0].Type
	if star, ok := expr.(*ast.StarExpr); ok {
		expr = star.X
	}
	switch e := expr.(type) {
	case *ast.IndexExpr:
		expr = e.X
	case *ast.IndexListExpr:
		expr = e.X
	}
	if id, ok := expr.(*ast.Ident); ok {
		return id.Name
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
