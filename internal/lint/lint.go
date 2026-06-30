package lint

import (
	"fmt"
	"go/ast"
	"go/token"
	"go/types"
	"path/filepath"
	"slices"
	"strings"
	"unicode"

	"github.com/mvrahden/go-test/internal/about"
	"golang.org/x/tools/go/analysis"
	"golang.org/x/tools/go/analysis/passes/inspect"
	"golang.org/x/tools/go/ast/inspector"
)

// Rule identifies a specific lint check.
type Rule string

const (
	Focus             Rule = "focus"
	Receiver          Rule = "receiver"
	LifecycleTypo     Rule = "lifecycle-typo"
	LifecyclePair     Rule = "lifecycle-pair"
	GeneratedFile     Rule = "generated-file"
	StdlibTest        Rule = "stdlib-test"
	Testify           Rule = "testify"
	PollScope         Rule = "poll-scope"
	TestSignature     Rule = "test-signature"
	XLifecycle        Rule = "x-lifecycle"
	AssertionSimplify Rule = "assertion-simplify"
	TEscape           Rule = "t-escape"
)

// SkippableRules is the set of rules that support opt-out via skip flags.
var SkippableRules = map[Rule]bool{
	StdlibTest: true,
	Testify:    true,
}

var cfg struct {
	skipStdlibTest bool
	skipTestify    bool
	disableNolint  bool
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
	Analyzer.Flags.BoolVar(&cfg.disableNolint, "disable-nolint", false, "report all diagnostics and let the analysis driver handle suppression")
}

var lifecycleHooks = []string{"BeforeAll", "AfterAll", "BeforeEach", "AfterEach"}

func run(pass *analysis.Pass) (any, error) {
	insp := pass.ResultOf[inspect.Analyzer].(*inspector.Inspector)

	suites := discoverSuites(insp)
	if len(suites) > 0 {
		checkMethods(pass, insp, suites)
		checkFocusPrefixes(pass, suites)
		checkLifecyclePairs(pass, suites)
	}

	checkOrphanedFiles(pass)
	checkStdlibTests(pass, insp)
	checkTestifyImports(pass)
	checkPollScope(pass, insp)
	checkAssertionSimplify(pass, insp)
	checkTEscape(pass, insp, suites)

	return nil, nil
}

func report(pass *analysis.Pass, rule Rule, pos token.Pos, format string, args ...any) {
	if !cfg.disableNolint && isSuppressed(pass, pos, rule) {
		return
	}
	pass.Report(analysis.Diagnostic{
		Pos:      pos,
		Category: string(rule),
		Message:  fmt.Sprintf(format, args...),
	})
}

func reportWithFix(pass *analysis.Pass, rule Rule, pos token.Pos, fixes []analysis.SuggestedFix, format string, args ...any) {
	if !cfg.disableNolint && isSuppressed(pass, pos, rule) {
		return
	}
	pass.Report(analysis.Diagnostic{
		Pos:            pos,
		Category:       string(rule),
		Message:        fmt.Sprintf(format, args...),
		SuggestedFixes: fixes,
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
	var rest string
	switch {
	case strings.HasPrefix(text, "//nolint"):
		rest = text[len("//nolint"):]
	case strings.HasPrefix(text, "// nolint"):
		rest = text[len("// nolint"):]
	default:
		return nil, false
	}
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
	name              string
	pos               token.Pos
	methods           map[string]token.Pos
	recvTypePositions []token.Pos
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
			stripped := strings.TrimPrefix(name, "F_")
			edits := []analysis.TextEdit{{
				Pos:     s.pos,
				End:     s.pos + 2,
				NewText: []byte(""),
			}}
			for _, p := range s.recvTypePositions {
				edits = append(edits, analysis.TextEdit{
					Pos:     p,
					End:     p + 2,
					NewText: []byte(""),
				})
			}
			reportWithFix(pass, Focus, s.pos,
				[]analysis.SuggestedFix{{
					Message:   fmt.Sprintf("rename %s to %s", name, stripped),
					TextEdits: edits,
				}},
				"focused suite %s should not be committed", name)
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

		if p := recvTypePos(fd.Recv); p != token.NoPos {
			suite.recvTypePositions = append(suite.recvTypePositions, p)
		}

		if !isPointerReceiver(fd.Recv) {
			reportWithFix(pass, Receiver, fd.Pos(),
				[]analysis.SuggestedFix{{
					Message: "use pointer receiver",
					TextEdits: []analysis.TextEdit{{
						Pos:     fd.Recv.List[0].Type.Pos(),
						End:     fd.Recv.List[0].Type.Pos(),
						NewText: []byte("*"),
					}},
				}},
				"suite method %s.%s should use a pointer receiver", recvName, methodName)
		}

		stripped := strings.TrimPrefix(strings.TrimPrefix(methodName, "F_"), "X_")
		if strings.HasPrefix(stripped, "Test") {
			if strings.HasPrefix(methodName, "F_") {
				reportWithFix(pass, Focus, fd.Pos(),
					[]analysis.SuggestedFix{{
						Message: fmt.Sprintf("rename %s to %s", methodName, strings.TrimPrefix(methodName, "F_")),
						TextEdits: []analysis.TextEdit{{
							Pos:     fd.Name.Pos(),
							End:     fd.Name.Pos() + 2,
							NewText: []byte(""),
						}},
					}},
					"focused method %s.%s should not be committed", recvName, methodName)
			}
			if !hasValidTestSignature(fd) {
				report(pass, TestSignature, fd.Pos(), "test method %s.%s has wrong signature — must accept *gotest.T", recvName, methodName)
			}
			return
		}

		if isLifecycleHook(stripped) {
			if strings.HasPrefix(methodName, "X_") {
				report(pass, XLifecycle, fd.Pos(), "X_ prefix on lifecycle hook %s.%s has no effect — remove the prefix or the method", recvName, methodName)
			}
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

// --- t-escape and suite rule detection ---

type escapeConfig struct {
	rule        Rule
	message     string
	suiteOnly   bool
	skipClosure bool
	canAutofix  bool
}

var escapeConfigs = map[string]escapeConfig{
	"Errorf":   {TEscape, "Errorf is available on gotest.T — unnecessary T escape", false, false, true},
	"FailNow":  {TEscape, "FailNow is available on gotest.T — unnecessary T escape", false, false, true},
	"Skipf":    {TEscape, "Skipf is available on gotest.T — unnecessary T escape", false, false, true},
	"Setenv":   {TEscape, "Setenv is available on gotest.T — unnecessary T escape", false, false, true},
	"TempDir":  {TEscape, "TempDir is available on gotest.T — unnecessary T escape", false, false, true},
	"Skip":     {TEscape, "Skipf is available on gotest.T — unnecessary T escape", false, false, false},
	"SkipNow":  {TEscape, "Skipf is available on gotest.T — unnecessary T escape", false, false, false},
	"Cleanup":  {TEscape, "use AfterEach or AfterAll for cleanup — T.Cleanup bypasses suite lifecycle", true, false, false},
	"Parallel": {TEscape, "use SuiteConfig.Parallel instead — T.Parallel bypasses suite lifecycle coordination", true, true, false},
	"Run":      {TEscape, "use It or When instead — T.Run bypasses gotest wrapping", true, true, false},
}

var gotestAssertionFuncs = map[string]bool{
	"True": true, "False": true,
	"Equal": true, "NotEqual": true,
	"Greater": true, "GreaterOrEqual": true,
	"Less": true, "LessOrEqual": true,
	"Zero": true, "NotZero": true,
	"Empty": true, "NotEmpty": true,
	"Len": true, "Contains": true, "NotContains": true,
	"NoError": true, "Error": true,
	"ErrorIs": true, "ErrorContains": true,
	"Regexp": true, "MatchSnapshot": true,
	"Eventually": true, "Consistently": true,
}

func checkTEscape(pass *analysis.Pass, insp *inspector.Inspector, suites map[string]*suiteInfo) {
	mr := buildMethodReach(pass, insp, 5)

	insp.Preorder([]ast.Node{(*ast.FuncDecl)(nil)}, func(n ast.Node) {
		fd := n.(*ast.FuncDecl)
		if fd.Body == nil {
			return
		}

		isSuiteMethod := false
		if fd.Recv != nil && len(fd.Recv.List) > 0 {
			recvName := receiverTypeName(fd.Recv)
			_, isSuiteMethod = suites[recvName]
		}

		tVars := map[string]bool{}
		gotestTVars := map[string]bool{}
		if isSuiteMethod && fd.Type.Params != nil && len(fd.Type.Params.List) > 0 {
			for _, name := range fd.Type.Params.List[0].Names {
				gotestTVars[name.Name] = true
			}
		}

		closureDepth := 0
		var stack []ast.Node

		ast.Inspect(fd.Body, func(n ast.Node) bool {
			if n == nil {
				top := stack[len(stack)-1]
				stack = stack[:len(stack)-1]
				if _, ok := top.(*ast.FuncLit); ok {
					closureDepth--
				}
				return false
			}
			stack = append(stack, n)
			if _, ok := n.(*ast.FuncLit); ok {
				closureDepth++
			}

			switch node := n.(type) {
			case *ast.AssignStmt:
				trackTVarAssign(node, tVars, gotestTVars)
			case *ast.CallExpr:
				reportEscape(pass, node, isSuiteMethod, closureDepth, tVars, gotestTVars, mr)
			}
			return true
		})
	})
}

func trackTVarAssign(assign *ast.AssignStmt, tVars, gotestTVars map[string]bool) {
	for i, rhs := range assign.Rhs {
		if i >= len(assign.Lhs) {
			break
		}
		lhsId, ok := assign.Lhs[i].(*ast.Ident)
		if !ok {
			continue
		}
		if isTMethodCall(rhs) {
			tVars[lhsId.Name] = true
			continue
		}
		if id, ok := rhs.(*ast.Ident); ok {
			if tVars[id.Name] {
				tVars[lhsId.Name] = true
			}
			if gotestTVars[id.Name] {
				gotestTVars[lhsId.Name] = true
			}
		}
	}
}

func reportEscape(pass *analysis.Pass, call *ast.CallExpr, isSuiteMethod bool, closureDepth int, tVars, gotestTVars map[string]bool, mr *methodReach) {
	sel, _ := call.Fun.(*ast.SelectorExpr)

	if sel != nil {
		if cfg, ok := escapeConfigs[sel.Sel.Name]; ok {
			if (!cfg.suiteOnly || isSuiteMethod) && (!cfg.skipClosure || closureDepth == 0) {
				isDirect := isTMethodCall(sel.X)
				isAlias := false
				if !isDirect {
					if id, ok := sel.X.(*ast.Ident); ok && tVars[id.Name] {
						isAlias = true
					}
				}
				if isDirect || isAlias {
					if cfg.canAutofix && isDirect {
						inner := sel.X.(*ast.CallExpr)
						innerSel := inner.Fun.(*ast.SelectorExpr)
						reportWithFix(pass, cfg.rule, call.Pos(),
							[]analysis.SuggestedFix{{
								Message: fmt.Sprintf("call %s directly", sel.Sel.Name),
								TextEdits: []analysis.TextEdit{{
									Pos:     innerSel.X.End(),
									End:     inner.End(),
									NewText: []byte(""),
								}},
							}},
							"%s", cfg.message)
					} else {
						report(pass, cfg.rule, call.Pos(), "%s", cfg.message)
					}
					return
				}
			}
		}

		if gotestAssertionFuncs[sel.Sel.Name] && isGotestPkgRef(pass, sel.X) && len(call.Args) > 0 {
			arg := call.Args[0]
			if inner, ok := arg.(*ast.CallExpr); ok && isTMethodCall(inner) {
				innerSel := inner.Fun.(*ast.SelectorExpr)
				reportWithFix(pass, TEscape, inner.Pos(),
					[]analysis.SuggestedFix{{
						Message: "pass gotest.T directly",
						TextEdits: []analysis.TextEdit{{
							Pos:     innerSel.X.End(),
							End:     inner.End(),
							NewText: []byte(""),
						}},
					}},
					"pass gotest.T directly to %s — unnecessary T escape", sel.Sel.Name)
				return
			}
			if id, ok := arg.(*ast.Ident); ok && tVars[id.Name] {
				report(pass, TEscape, arg.Pos(),
					"pass gotest.T directly to %s — unnecessary T escape", sel.Sel.Name)
				return
			}
		}
	}

	if isSuiteMethod {
		methods := mr.reachedMethods(call, tVars, gotestTVars)
		reported := map[Rule]bool{}
		for method := range methods {
			cfg := escapeConfigs[method]
			if !cfg.suiteOnly {
				continue
			}
			if cfg.skipClosure && closureDepth > 0 {
				continue
			}
			if reported[cfg.rule] {
				continue
			}
			reported[cfg.rule] = true
			report(pass, cfg.rule, call.Pos(), "%s", cfg.message)
		}
	}
}

func isGotestPkgRef(pass *analysis.Pass, expr ast.Expr) bool {
	id, ok := expr.(*ast.Ident)
	if !ok {
		return false
	}
	obj := pass.TypesInfo.Uses[id]
	if obj == nil {
		return false
	}
	pkgName, ok := obj.(*types.PkgName)
	if !ok {
		return false
	}
	return pkgName.Imported().Path() == "github.com/mvrahden/go-test/pkg/gotest"
}

// --- interprocedural method reachability ---

// methodReach tracks which function parameters transitively lead to calls
// of flagged methods, enabling interprocedural detection across helper chains.
type methodReach struct {
	pass      *analysis.Pass
	funcDecls map[types.Object]*ast.FuncDecl
	params    map[*ast.FuncDecl]map[int]map[string]bool
}

func buildMethodReach(pass *analysis.Pass, insp *inspector.Inspector, maxDepth int) *methodReach {
	mr := &methodReach{
		pass:      pass,
		funcDecls: map[types.Object]*ast.FuncDecl{},
		params:    map[*ast.FuncDecl]map[int]map[string]bool{},
	}

	insp.Preorder([]ast.Node{(*ast.FuncDecl)(nil)}, func(n ast.Node) {
		fd := n.(*ast.FuncDecl)
		if fd.Body == nil || fd.Name == nil {
			return
		}
		if obj := pass.TypesInfo.Defs[fd.Name]; obj != nil {
			mr.funcDecls[obj] = fd
		}
	})

	for _, fd := range mr.funcDecls {
		mr.scanDirect(fd)
	}
	for round := range maxDepth {
		_ = round
		changed := false
		for _, fd := range mr.funcDecls {
			if mr.propagate(fd) {
				changed = true
			}
		}
		if !changed {
			break
		}
	}

	return mr
}

func (mr *methodReach) mark(fd *ast.FuncDecl, paramIdx int, method string) bool {
	if mr.params[fd] == nil {
		mr.params[fd] = map[int]map[string]bool{}
	}
	if mr.params[fd][paramIdx] == nil {
		mr.params[fd][paramIdx] = map[string]bool{}
	}
	if mr.params[fd][paramIdx][method] {
		return false
	}
	mr.params[fd][paramIdx][method] = true
	return true
}

func (mr *methodReach) resolveCallee(call *ast.CallExpr) *ast.FuncDecl {
	var ident *ast.Ident
	switch fn := call.Fun.(type) {
	case *ast.Ident:
		ident = fn
	case *ast.SelectorExpr:
		ident = fn.Sel
	}
	if ident == nil {
		return nil
	}
	obj := mr.pass.TypesInfo.Uses[ident]
	if obj == nil {
		return nil
	}
	return mr.funcDecls[obj]
}

func (mr *methodReach) scanDirect(fd *ast.FuncDecl) {
	aliases := flattenParams(fd.Type.Params)
	if len(aliases) == 0 {
		return
	}

	ast.Inspect(fd.Body, func(n ast.Node) bool {
		switch node := n.(type) {
		case *ast.AssignStmt:
			trackParamFlow(node, aliases)
		case *ast.CallExpr:
			sel, ok := node.Fun.(*ast.SelectorExpr)
			if !ok {
				break
			}
			if _, ok := escapeConfigs[sel.Sel.Name]; !ok {
				break
			}
			method := sel.Sel.Name
			if id, ok := sel.X.(*ast.Ident); ok {
				if idx, ok := aliases[id.Name]; ok {
					mr.mark(fd, idx, method)
				}
			}
			if isTMethodCall(sel.X) {
				innerSel := sel.X.(*ast.CallExpr).Fun.(*ast.SelectorExpr)
				if id, ok := innerSel.X.(*ast.Ident); ok {
					if idx, ok := aliases[id.Name]; ok {
						mr.mark(fd, idx, method)
					}
				}
			}
		}
		return true
	})
}

func (mr *methodReach) propagate(fd *ast.FuncDecl) bool {
	aliases := flattenParams(fd.Type.Params)
	if len(aliases) == 0 {
		return false
	}

	changed := false
	ast.Inspect(fd.Body, func(n ast.Node) bool {
		switch node := n.(type) {
		case *ast.AssignStmt:
			trackParamFlow(node, aliases)
		case *ast.CallExpr:
			callee := mr.resolveCallee(node)
			if callee == nil {
				break
			}
			calleeReach := mr.params[callee]
			if len(calleeReach) == 0 {
				break
			}
			for argIdx, arg := range node.Args {
				methods := calleeReach[argIdx]
				if len(methods) == 0 {
					continue
				}
				if idx := exprToParamIdx(arg, aliases); idx >= 0 {
					for method := range methods {
						if mr.mark(fd, idx, method) {
							changed = true
						}
					}
				}
			}
		}
		return true
	})
	return changed
}

func (mr *methodReach) reachedMethods(call *ast.CallExpr, tVars, gotestTVars map[string]bool) map[string]bool {
	callee := mr.resolveCallee(call)
	if callee == nil {
		return nil
	}
	calleeReach := mr.params[callee]
	if len(calleeReach) == 0 {
		return nil
	}
	var methods map[string]bool
	for argIdx, arg := range call.Args {
		argMethods := calleeReach[argIdx]
		if len(argMethods) == 0 {
			continue
		}
		tainted := isTMethodCall(arg)
		if !tainted {
			if id, ok := arg.(*ast.Ident); ok {
				tainted = tVars[id.Name] || gotestTVars[id.Name]
			}
		}
		if tainted {
			if methods == nil {
				methods = map[string]bool{}
			}
			for m := range argMethods {
				methods[m] = true
			}
		}
	}
	return methods
}

// flattenParams returns a map from parameter name to its flattened index.
func flattenParams(params *ast.FieldList) map[string]int {
	if params == nil {
		return nil
	}
	m := map[string]int{}
	idx := 0
	for _, field := range params.List {
		for _, name := range field.Names {
			m[name.Name] = idx
			idx++
		}
	}
	return m
}

// trackParamFlow extends the alias map for direct assignments (x := param)
// and .T() calls (x := param.T()).
func trackParamFlow(assign *ast.AssignStmt, aliases map[string]int) {
	for i, rhs := range assign.Rhs {
		if i >= len(assign.Lhs) {
			break
		}
		lhsId, ok := assign.Lhs[i].(*ast.Ident)
		if !ok {
			continue
		}
		if id, ok := rhs.(*ast.Ident); ok {
			if idx, ok := aliases[id.Name]; ok {
				aliases[lhsId.Name] = idx
			}
			continue
		}
		if call, ok := rhs.(*ast.CallExpr); ok && isTMethodCall(call) {
			if sel, ok := call.Fun.(*ast.SelectorExpr); ok {
				if id, ok := sel.X.(*ast.Ident); ok {
					if idx, ok := aliases[id.Name]; ok {
						aliases[lhsId.Name] = idx
					}
				}
			}
		}
	}
}

// exprToParamIdx returns the parameter index if the expression is a
// parameter/alias ident or a .T() call on one. Returns -1 otherwise.
func exprToParamIdx(expr ast.Expr, aliases map[string]int) int {
	if id, ok := expr.(*ast.Ident); ok {
		if idx, ok := aliases[id.Name]; ok {
			return idx
		}
	}
	if call, ok := expr.(*ast.CallExpr); ok && isTMethodCall(call) {
		if sel, ok := call.Fun.(*ast.SelectorExpr); ok {
			if id, ok := sel.X.(*ast.Ident); ok {
				if idx, ok := aliases[id.Name]; ok {
					return idx
				}
			}
		}
	}
	return -1
}

func isTMethodCall(expr ast.Expr) bool {
	call, ok := expr.(*ast.CallExpr)
	if !ok || len(call.Args) != 0 {
		return false
	}
	sel, ok := call.Fun.(*ast.SelectorExpr)
	return ok && sel.Sel.Name == "T"
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
		if !cfg.disableNolint && docSuppressed(fd.Doc, StdlibTest) {
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

func hasValidTestSignature(fd *ast.FuncDecl) bool {
	params := fd.Type.Params
	if params == nil || len(params.List) < 1 || len(params.List) > 2 {
		return false
	}
	return isSupportedT(params.List[0].Type)
}

func isSupportedT(expr ast.Expr) bool {
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
	return sel.Sel.Name == "T" && (ident.Name == "gotest" || ident.Name == "testing")
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

func recvTypePos(recv *ast.FieldList) token.Pos {
	t := recv.List[0].Type
	if star, ok := t.(*ast.StarExpr); ok {
		t = star.X
	}
	switch x := t.(type) {
	case *ast.Ident:
		return x.Pos()
	case *ast.IndexExpr:
		if ident, ok := x.X.(*ast.Ident); ok {
			return ident.Pos()
		}
	case *ast.IndexListExpr:
		if ident, ok := x.X.(*ast.Ident); ok {
			return ident.Pos()
		}
	}
	return token.NoPos
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
