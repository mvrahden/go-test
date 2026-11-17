package gotestast

import (
	"fmt"
	"go/ast"
	"go/token"
	"go/types"
	"strings"

	"github.com/mvrahden/go-test/internal/typeclass"
)

// HookUsesReceiver reports whether a hook's body names its receiver. A hook
// whose source is not at hand (decl nil) is taken to.
func HookUsesReceiver(decl *ast.FuncDecl, info *types.Info) bool {
	if decl == nil || decl.Body == nil || info == nil {
		return true
	}
	if decl.Recv == nil || len(decl.Recv.List) == 0 || len(decl.Recv.List[0].Names) == 0 {
		return false
	}
	name := decl.Recv.List[0].Names[0]
	if name.Name == "_" {
		return false
	}
	self := info.Defs[name]
	if self == nil {
		return true
	}
	uses := false
	ast.Inspect(decl.Body, func(n ast.Node) bool {
		if id, ok := n.(*ast.Ident); ok && info.Uses[id] == self {
			uses = true
		}
		return !uses
	})
	return uses
}

// ParallelFixtureHooks refuses a suite with Parallel: true and two or more
// test methods that binds fixture, whose named per-method hooks use it.
func ParallelFixtureHooks(s *TestSuiteSpec, fixture string, hooks []string) error {
	if !s.IsMethodParallel() || s.RunnableTests() < 2 || len(hooks) == 0 {
		return nil
	}
	verb, hook := "uses", "hook"
	if len(hooks) > 1 {
		verb, hook = "use", "hooks"
	}
	return fmt.Errorf("suite %s has Parallel: true, but %s of fixture %s %s the fixture, which the suite's parallel methods share: the %s would run for all of them at once — do the per-method work in the suite's returning BeforeEach, or drop Parallel", s.Identifier(), strings.Join(hooks, " and "), fixture, verb, hook)
}

// parallelSetenv refuses a parallel suite whose test method, BeforeEach or
// AfterEach calls Setenv on a T: testing panics there. It reads each
// method's own body, closures included, not the helpers it calls.
func parallelSetenv(ts *TestSuiteSpec) error {
	if !ts.th.ConfigParallel {
		return nil
	}
	methods := []*TestSuiteMethod{ts.th.BeforeEach, ts.th.AfterEach}
	for _, tc := range ts.th.TestCases {
		if !tc.IsExcluded() {
			methods = append(methods, tc)
		}
	}
	first, method := token.NoPos, ""
	for _, m := range methods {
		if pos := m.callsSetenv(); pos.IsValid() && (!first.IsValid() || pos < first) {
			first, method = pos, m.m.Name()
		}
	}
	if !first.IsValid() {
		return nil
	}
	return At(first, fmt.Errorf("%s.%s calls Setenv, but the suite has Parallel: true and testing panics when a parallel test sets the environment; set it in BeforeAll, or drop Parallel", ts.ts.Name.Name, method))
}

// callsSetenv returns the position of the first Setenv selector in the
// method's body called on a *gotest.T or a *testing.T, or token.NoPos.
func (m *TestSuiteMethod) callsSetenv() token.Pos {
	decl, ok := m.node().(*ast.FuncDecl)
	if !ok || decl.Body == nil || m.info == nil {
		return token.NoPos
	}
	found := token.NoPos
	ast.Inspect(decl.Body, func(n ast.Node) bool {
		if found.IsValid() {
			return false
		}
		call, ok := n.(*ast.CallExpr)
		if !ok {
			return true
		}
		sel, ok := call.Fun.(*ast.SelectorExpr)
		if !ok || sel.Sel.Name != "Setenv" {
			return true
		}
		fn, ok := m.info.Uses[sel.Sel].(*types.Func)
		if !ok || fn.Signature().Recv() == nil {
			return true
		}
		if h, _ := typeclass.CaseHandle(fn.Signature().Recv().Type()); h == typeclass.TestHandle {
			found = sel.Sel.Pos()
		}
		return true
	})
	return found
}
