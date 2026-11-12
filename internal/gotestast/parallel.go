package gotestast

import (
	"fmt"
	"go/ast"
	"go/types"
	"strings"
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
