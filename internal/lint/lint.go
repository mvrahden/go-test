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
	"github.com/mvrahden/go-test/internal/gotestast"
	"github.com/mvrahden/go-test/internal/protocol"
	"golang.org/x/tools/go/analysis"
	"golang.org/x/tools/go/analysis/passes/inspect"
	"golang.org/x/tools/go/ast/inspector"
)

// Rule identifies a specific lint check.
type Rule string

const (
	Focus              Rule = "focus"
	Receiver           Rule = "receiver"
	LifecycleTypo      Rule = "lifecycle-typo"
	LifecyclePair      Rule = "lifecycle-pair"
	GeneratedFile      Rule = "generated-file"
	StdlibTest         Rule = "stdlib-test"
	Testify            Rule = "testify"
	PollScope          Rule = "poll-scope"
	TestSignature      Rule = "test-signature"
	XLifecycle         Rule = "x-lifecycle"
	AssertionSimplify  Rule = "assertion-simplify"
	FailGuard          Rule = "fail-guard"
	AssertionTypeGuard Rule = "assertion-type-guard"
	AssertionRedundant Rule = "assertion-redundant"
	TEscape            Rule = "t-escape"
	SuiteLifecycle     Rule = "suite-lifecycle"
	// SharedFixtureUndeclared is integrity: window scheduling starts only
	// the fixtures scheduled suites declare, so an undeclared read may hit
	// a fixture that never started or is already released.
	SharedFixtureUndeclared Rule = "shared-fixture-undeclared"
)

// Tier classifies what breaks when a rule's finding is ignored, and derives
// the suppression policy: integrity rules (test outcomes can lie, resources
// can leak) are suppressible per line only; expressiveness rules (the test is
// correct but says it worse) and migration rules (legitimate coexistence) may
// additionally be skipped project-wide.
type Tier int

const (
	TierIntegrity Tier = iota
	TierExpressiveness
	TierMigration
)

// Scope documents where a rule may fire.
type Scope int

const (
	ScopeEverywhere Scope = iota
	ScopeGotestFiles
	ScopeSuites
	ScopePollCallbacks
)

var ruleMeta = map[Rule]struct {
	Tier  Tier
	Scope Scope
}{
	Focus:              {TierIntegrity, ScopeSuites},
	Receiver:           {TierIntegrity, ScopeSuites},
	LifecycleTypo:      {TierIntegrity, ScopeSuites},
	LifecyclePair:      {TierIntegrity, ScopeSuites},
	GeneratedFile:      {TierIntegrity, ScopeEverywhere},
	StdlibTest:         {TierMigration, ScopeEverywhere},
	Testify:            {TierMigration, ScopeEverywhere},
	PollScope:          {TierIntegrity, ScopePollCallbacks},
	TestSignature:      {TierIntegrity, ScopeSuites},
	XLifecycle:         {TierIntegrity, ScopeSuites},
	AssertionSimplify:  {TierExpressiveness, ScopeGotestFiles},
	AssertionTypeGuard: {TierIntegrity, ScopeGotestFiles},
	AssertionRedundant: {TierExpressiveness, ScopeGotestFiles},
	TEscape:            {TierExpressiveness, ScopeSuites},
	SuiteLifecycle:     {TierIntegrity, ScopeSuites},
	FailGuard:          {TierExpressiveness, ScopeGotestFiles},

	SharedFixtureUndeclared: {TierIntegrity, ScopeSuites},
}

// Known reports whether the rule ID exists.
func Known(r Rule) bool {
	_, ok := ruleMeta[r]
	return ok
}

// SkippableRules is derived from the tier table: every non-integrity rule
// supports opt-out via a skip flag (and .gotest.yml lint.skip).
var SkippableRules = func() map[Rule]bool {
	m := make(map[Rule]bool, len(ruleMeta))
	for r, meta := range ruleMeta {
		if meta.Tier != TierIntegrity {
			m[r] = true
		}
	}
	return m
}()

var cfg struct {
	skip          map[Rule]*bool
	disableNolint bool
}

var Analyzer = &analysis.Analyzer{
	Name:     "gotestlint",
	Doc:      "checks for common mistakes in gotest test suites",
	Requires: []*analysis.Analyzer{inspect.Analyzer},
	Run:      run,
}

func init() {
	cfg.skip = make(map[Rule]*bool, len(SkippableRules))
	rules := make([]string, 0, len(SkippableRules))
	for r := range SkippableRules {
		rules = append(rules, string(r))
	}
	slices.Sort(rules)
	for _, r := range rules {
		cfg.skip[Rule(r)] = Analyzer.Flags.Bool("skip-"+r, false, "disable the "+r+" rule")
	}
	Analyzer.Flags.BoolVar(&cfg.disableNolint, "disable-nolint", false, "report all diagnostics and let the analysis driver handle suppression")
}

func skipped(rule Rule) bool {
	b, ok := cfg.skip[rule]
	return ok && *b
}

var lifecycleHooks = []string{"BeforeAll", "AfterAll", "BeforeEach", "AfterEach"}

func run(pass *analysis.Pass) (any, error) {
	insp := pass.ResultOf[inspect.Analyzer].(*inspector.Inspector)

	suites := discoverSuites(insp)
	if len(suites) > 0 {
		checkMethods(pass, insp, suites)
		checkFocusPrefixes(pass, suites)
		checkLifecyclePairs(pass, suites)
		checkSharedFixtureUndeclared(pass, insp, suites)
	}

	checkOrphanedFiles(pass)
	checkStdlibTests(pass, insp)
	checkTestifyImports(pass)

	// Integrity rules run first and claim the constructs they own;
	// expressiveness rules stand down on claimed spans so one construct
	// yields one finding, from the rule with the stronger story.
	cl := &claims{}
	checkPollScope(pass, insp, cl)
	checkTEscape(pass, insp, suites, cl)
	checkAssertionSimplify(pass, insp, cl)
	checkFailGuard(pass, insp, cl)
	checkRedundantAssertion(pass, insp, cl)

	return nil, nil
}

// claims records positions owned by earlier findings, whether or not a
// diagnostic was shown — per-line suppression exempts the construct, not
// just the message. A skipped expressiveness rule releases its constructs
// to the remaining active rules; integrity rules cannot be skipped and
// always claim.
type claims struct{ positions []token.Pos }

func (c *claims) add(rule Rule, pos token.Pos) {
	if ruleMeta[rule].Tier != TierIntegrity && skipped(rule) {
		return
	}
	c.positions = append(c.positions, pos)
}

func (c *claims) anyWithin(start, end token.Pos) bool {
	for _, p := range c.positions {
		if p >= start && p < end {
			return true
		}
	}
	return false
}

func report(pass *analysis.Pass, rule Rule, pos token.Pos, format string, args ...any) {
	if skipped(rule) {
		return
	}
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
	if skipped(rule) {
		return
	}
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

// fileContaining returns the syntax file whose span contains pos, or nil.
func fileContaining(pass *analysis.Pass, pos token.Pos) *ast.File {
	for _, file := range pass.Files {
		if pos >= file.FileStart && pos <= file.FileEnd {
			return file
		}
	}
	return nil
}

// nolintAliases maps a rule to the historical umbrella ID whose nolint
// comments also suppress it: suite-lifecycle was split out of t-escape, and
// committed //nolint:t-escape comments must keep working.
var nolintAliases = map[Rule]Rule{
	SuiteLifecycle: TEscape,
}

func ruleMatched(rules map[Rule]bool, rule Rule) bool {
	if rules == nil || rules[rule] {
		return true
	}
	umbrella, ok := nolintAliases[rule]
	return ok && rules[umbrella]
}

func isSuppressed(pass *analysis.Pass, pos token.Pos, rule Rule) bool {
	file := fileContaining(pass, pos)
	if file == nil {
		return false
	}
	line := pass.Fset.Position(pos).Line
	pkgLine := pass.Fset.Position(file.Package).Line
	for _, cg := range file.Comments {
		for _, c := range cg.List {
			rules, ok := parseNolint(c.Text)
			if !ok {
				continue
			}
			if !ruleMatched(rules, rule) {
				continue
			}
			cLine := pass.Fset.Position(c.Pos()).Line
			if cLine == pkgLine || cLine == line {
				return true
			}
			if pass.Fset.Position(cg.End()).Line == line-1 && startsItsLine(pass, file, cg) {
				return true
			}
		}
	}
	return false
}

// startsItsLine reports whether no code precedes the comment group on its
// opening line — a comment trailing the previous statement must not suppress
// the line below it.
func startsItsLine(pass *analysis.Pass, file *ast.File, cg *ast.CommentGroup) bool {
	cgLine := pass.Fset.Position(cg.Pos()).Line
	standalone := true
	ast.Inspect(file, func(n ast.Node) bool {
		if n == nil || !standalone {
			return false
		}
		if n.End() <= cg.Pos() && pass.Fset.Position(n.End()).Line == cgLine {
			standalone = false
			return false
		}
		return n.Pos() < cg.Pos()
	})
	return standalone
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
			stripped := strings.TrimPrefix(strings.TrimPrefix(name, protocol.PrefixFocused), protocol.PrefixExcluded)
			if strings.HasSuffix(stripped, protocol.SuffixTestSuite) {
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
		if strings.HasPrefix(name, protocol.PrefixFocused) {
			stripped := strings.TrimPrefix(name, protocol.PrefixFocused)
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

		stripped := strings.TrimPrefix(strings.TrimPrefix(methodName, protocol.PrefixFocused), protocol.PrefixExcluded)
		if strings.HasPrefix(stripped, "Test") {
			if strings.HasPrefix(methodName, protocol.PrefixFocused) {
				reportWithFix(pass, Focus, fd.Pos(),
					[]analysis.SuggestedFix{{
						Message: fmt.Sprintf("rename %s to %s", methodName, strings.TrimPrefix(methodName, protocol.PrefixFocused)),
						TextEdits: []analysis.TextEdit{{
							Pos:     fd.Name.Pos(),
							End:     fd.Name.Pos() + 2,
							NewText: []byte(""),
						}},
					}},
					"focused method %s.%s should not be committed", recvName, methodName)
			}
			if !hasValidTestSignature(fd) {
				report(pass, TestSignature, fd.Pos(), "test method %s.%s has wrong signature — must accept *gotest.T (or *testing.T)", recvName, methodName)
			}
			return
		}

		if isLifecycleHook(stripped) {
			if strings.HasPrefix(methodName, protocol.PrefixExcluded) {
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
	rule       Rule
	message    string
	suiteOnly  bool
	canAutofix bool
	directOnly bool // only flag direct t.T().X() calls; plain *testing.T helpers may use X legitimately
}

var escapeConfigs = map[string]escapeConfig{
	"Errorf":   {TEscape, "Errorf is available on gotest.T — unnecessary T escape", false, true, false},
	"FailNow":  {TEscape, "FailNow is available on gotest.T — unnecessary T escape", false, true, false},
	"Skipf":    {TEscape, "Skipf is available on gotest.T — unnecessary T escape", false, true, false},
	"Setenv":   {TEscape, "Setenv is available on gotest.T — unnecessary T escape", false, true, false},
	"TempDir":  {TEscape, "TempDir is available on gotest.T — unnecessary T escape", false, true, false},
	"Skip":     {TEscape, "must use Skipf instead — unnecessary T escape", false, false, false},
	"SkipNow":  {TEscape, "must use Skipf instead — unnecessary T escape", false, false, false},
	"Cleanup":  {SuiteLifecycle, "use AfterEach or AfterAll for cleanup — T.Cleanup bypasses suite lifecycle", true, false, false},
	"Parallel": {SuiteLifecycle, "use SuiteConfig.Parallel instead — T.Parallel bypasses suite lifecycle coordination", true, false, false},
	"Run":      {SuiteLifecycle, "use It or When instead — T.Run bypasses gotest wrapping", true, false, false},
	"Helper":   {TEscape, "never call Helper — gotest resolves call sites automatically; Helper degrades failure locations", false, false, true},
	"Log":      {TEscape, "use assertion message args instead — T.Log bypasses the failure report", false, false, true},
	"Fatal":    {TEscape, "use assertions instead — T.Fatal bypasses the assertion tracer", false, false, true},
	"Fatalf":   {TEscape, "use assertions instead — T.Fatalf bypasses the assertion tracer", false, false, true},
}

type deferredReport struct {
	rule    Rule
	pos     token.Pos
	message string
}

func checkTEscape(pass *analysis.Pass, insp *inspector.Inspector, suites map[string]*suiteInfo, cl *claims) {
	mr := buildMethodReach(pass, insp, 5)
	reported := map[token.Pos]bool{}
	var deferred []deferredReport

	scanBody := func(body *ast.BlockStmt, isSuiteMethod bool, gotestTVars map[string]bool) {
		tVars := map[string]bool{}
		ast.Inspect(body, func(n ast.Node) bool {
			switch node := n.(type) {
			case *ast.AssignStmt:
				trackTVarAssign(node, tVars, gotestTVars)
			case *ast.CallExpr:
				reportDirectEscape(pass, cl, node, isSuiteMethod, tVars, reported)
				if isSuiteMethod {
					collectInterproceduralEscape(node, tVars, gotestTVars, mr, &deferred)
				}
			}
			return true
		})
	}

	insp.Preorder([]ast.Node{(*ast.FuncDecl)(nil)}, func(n ast.Node) {
		fd := n.(*ast.FuncDecl)
		if fd.Body == nil {
			return
		}

		isSuiteMethod := false
		if fd.Recv != nil && len(fd.Recv.List) > 0 {
			_, isSuiteMethod = suites[receiverTypeName(fd.Recv)]
		}

		gotestTVars := map[string]bool{}
		if isSuiteMethod && fd.Type.Params != nil {
			for _, field := range fd.Type.Params.List {
				if isGotestTType(pass, field) {
					for _, name := range field.Names {
						gotestTVars[name.Name] = true
					}
				}
			}
		}

		scanBody(fd.Body, isSuiteMethod, gotestTVars)
	})

	// Package-level var function literals are not FuncDecls but their bodies
	// escape t.T() the same way — without scanning them the claims table
	// would have blind spots the expressiveness rules rely on.
	for _, file := range pass.Files {
		for _, decl := range file.Decls {
			gd, ok := decl.(*ast.GenDecl)
			if !ok || gd.Tok != token.VAR {
				continue
			}
			for _, spec := range gd.Specs {
				vs, ok := spec.(*ast.ValueSpec)
				if !ok {
					continue
				}
				for _, value := range vs.Values {
					if lit, ok := value.(*ast.FuncLit); ok && lit.Body != nil {
						scanBody(lit.Body, false, map[string]bool{})
					}
				}
			}
		}
	}

	for _, d := range deferred {
		if !reported[d.pos] {
			reported[d.pos] = true
			cl.add(d.rule, d.pos)
			report(pass, d.rule, d.pos, "%s", d.message)
		}
	}
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

func reportDirectEscape(pass *analysis.Pass, cl *claims, call *ast.CallExpr, isSuiteMethod bool, tVars map[string]bool, reported map[token.Pos]bool) {
	sel, _ := call.Fun.(*ast.SelectorExpr)
	if sel == nil {
		return
	}

	if esc, ok := escapeConfigs[sel.Sel.Name]; ok {
		if !esc.suiteOnly || isSuiteMethod {
			isDirect := isTMethodCall(sel.X)
			isAlias := false
			if !isDirect {
				if id, ok := sel.X.(*ast.Ident); ok && tVars[id.Name] {
					isAlias = true
				}
			}
			if isDirect || isAlias {
				reported[call.Pos()] = true
				cl.add(esc.rule, call.Pos())
				if esc.canAutofix && isDirect {
					inner := sel.X.(*ast.CallExpr)
					innerSel := inner.Fun.(*ast.SelectorExpr)
					reportWithFix(pass, esc.rule, call.Pos(),
						[]analysis.SuggestedFix{{
							Message: fmt.Sprintf("call %s directly", sel.Sel.Name),
							TextEdits: []analysis.TextEdit{{
								Pos:     innerSel.X.End(),
								End:     inner.End(),
								NewText: []byte(""),
							}},
						}},
						"%s", esc.message)
				} else {
					report(pass, esc.rule, call.Pos(), "%s", esc.message)
				}
				return
			}
		}
	}

	if assertionFuncName(pass, call.Fun) != "" && len(call.Args) > 0 {
		arg := call.Args[0]
		if inner, ok := arg.(*ast.CallExpr); ok && isTMethodCall(inner) {
			reported[inner.Pos()] = true
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
			reported[arg.Pos()] = true
			report(pass, TEscape, arg.Pos(),
				"pass gotest.T directly to %s — unnecessary T escape", sel.Sel.Name)
			return
		}
	}
}

func collectInterproceduralEscape(call *ast.CallExpr, tVars, gotestTVars map[string]bool, mr *methodReach, results *[]deferredReport) {
	methods := mr.reachedMethods(call, tVars, gotestTVars)
	for method, positions := range methods {
		esc := escapeConfigs[method]
		for pos := range positions {
			*results = append(*results, deferredReport{esc.rule, pos, esc.message})
		}
	}
}

var gotestImportPath = about.Repo + "/pkg/gotest"

// assertionFuncName resolves fun to an exported gotest function whose first
// parameter is the package's testingT interface — the assertion shape — and
// returns its name. Deriving the surface from type information means the
// linter can never drift from the API, and lookalike names from other
// packages never match.
func assertionFuncName(pass *analysis.Pass, fun ast.Expr) string {
	var id *ast.Ident
	switch fn := fun.(type) {
	case *ast.SelectorExpr:
		id = fn.Sel
	case *ast.Ident:
		id = fn
	case *ast.IndexExpr:
		return assertionFuncName(pass, fn.X)
	case *ast.IndexListExpr:
		return assertionFuncName(pass, fn.X)
	default:
		return ""
	}
	fn, ok := pass.TypesInfo.Uses[id].(*types.Func)
	if !ok || fn.Pkg() == nil || fn.Pkg().Path() != gotestImportPath {
		return ""
	}
	sig, ok := fn.Type().(*types.Signature)
	if !ok || sig.Params().Len() == 0 {
		return ""
	}
	named, ok := sig.Params().At(0).Type().(*types.Named)
	if !ok || named.Obj().Name() != "testingT" {
		return ""
	}
	return fn.Name()
}

func isGotestTType(pass *analysis.Pass, field *ast.Field) bool {
	return namedPtrType(pass.TypesInfo.TypeOf(field.Type), gotestImportPath, "T")
}

// --- interprocedural method reachability ---

type callEdge struct {
	callee *ast.FuncDecl
	args   map[int]int // call arg position → caller's param index
}

// methodReach tracks which function parameters transitively lead to calls
// of flagged methods, enabling interprocedural detection across helper chains.
type methodReach struct {
	pass      *analysis.Pass
	funcDecls map[types.Object]*ast.FuncDecl
	params    map[*ast.FuncDecl]map[int]map[string]map[token.Pos]bool
	edges     map[*ast.FuncDecl][]callEdge
}

func buildMethodReach(pass *analysis.Pass, insp *inspector.Inspector, maxDepth int) *methodReach {
	mr := &methodReach{
		pass:      pass,
		funcDecls: map[types.Object]*ast.FuncDecl{},
		params:    map[*ast.FuncDecl]map[int]map[string]map[token.Pos]bool{},
		edges:     map[*ast.FuncDecl][]callEdge{},
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
	for range maxDepth {
		if !mr.propagate() {
			break
		}
	}

	return mr
}

func (mr *methodReach) mark(fd *ast.FuncDecl, paramIdx int, method string, pos token.Pos) bool {
	if mr.params[fd] == nil {
		mr.params[fd] = map[int]map[string]map[token.Pos]bool{}
	}
	if mr.params[fd][paramIdx] == nil {
		mr.params[fd][paramIdx] = map[string]map[token.Pos]bool{}
	}
	if mr.params[fd][paramIdx][method] == nil {
		mr.params[fd][paramIdx][method] = map[token.Pos]bool{}
	}
	if mr.params[fd][paramIdx][method][pos] {
		return false
	}
	mr.params[fd][paramIdx][method][pos] = true
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
			if ok {
				if esc, ok := escapeConfigs[sel.Sel.Name]; ok && !esc.directOnly {
					method := sel.Sel.Name
					if id, ok := sel.X.(*ast.Ident); ok {
						if idx, ok := aliases[id.Name]; ok {
							mr.mark(fd, idx, method, node.Pos())
						}
					}
					if isTMethodCall(sel.X) {
						innerSel := sel.X.(*ast.CallExpr).Fun.(*ast.SelectorExpr)
						if id, ok := innerSel.X.(*ast.Ident); ok {
							if idx, ok := aliases[id.Name]; ok {
								mr.mark(fd, idx, method, node.Pos())
							}
						}
					}
				}
			}
			if callee := mr.resolveCallee(node); callee != nil {
				edge := callEdge{callee: callee, args: map[int]int{}}
				for argIdx, arg := range node.Args {
					if idx := exprToParamIdx(arg, aliases); idx >= 0 {
						edge.args[argIdx] = idx
					}
				}
				if len(edge.args) > 0 {
					mr.edges[fd] = append(mr.edges[fd], edge)
				}
			}
		}
		return true
	})
}

func (mr *methodReach) propagate() bool {
	changed := false
	for fd, edges := range mr.edges {
		for _, edge := range edges {
			calleeReach := mr.params[edge.callee]
			if len(calleeReach) == 0 {
				continue
			}
			for argIdx, callerParamIdx := range edge.args {
				for method, positions := range calleeReach[argIdx] {
					for pos := range positions {
						if mr.mark(fd, callerParamIdx, method, pos) {
							changed = true
						}
					}
				}
			}
		}
	}
	return changed
}

func (mr *methodReach) reachedMethods(call *ast.CallExpr, tVars, gotestTVars map[string]bool) map[string]map[token.Pos]bool {
	callee := mr.resolveCallee(call)
	if callee == nil {
		return nil
	}
	calleeReach := mr.params[callee]
	if len(calleeReach) == 0 {
		return nil
	}
	var methods map[string]map[token.Pos]bool
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
				methods = map[string]map[token.Pos]bool{}
			}
			for method, positions := range argMethods {
				if methods[method] == nil {
					methods[method] = map[token.Pos]bool{}
				}
				for pos := range positions {
					methods[method][pos] = true
				}
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
		if len(field.Names) == 0 {
			idx++
			continue
		}
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
	if skipped(StdlibTest) {
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
	if skipped(Testify) {
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
	return gotestast.ReceiverTypeName(recv.List[0].Type)
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

// foreignAssertionNames is deliberately syntactic and package-agnostic:
// poll-scope is an integrity rule, and an assertion from a foreign library
// (testify et al.) escaping the poll loop is exactly as broken as a gotest
// one. This vocabulary cannot be derived from gotest's type information.
var foreignAssertionNames = map[string]bool{
	"Consistently": true, "Contains": true, "ElementsMatch": true,
	"Empty": true, "Equal": true, "Error": true,
	"ErrorAs": true, "ErrorContains": true, "ErrorIs": true,
	"Eventually": true, "Fail": true, "FailNow": true,
	"False": true, "Greater": true, "GreaterOrEqual": true,
	"InDelta": true, "JSONEq": true, "Len": true,
	"Less": true, "LessOrEqual": true, "Nil": true,
	"NoError": true, "NotContains": true, "NotEmpty": true,
	"NotEqual": true, "NotNil": true, "NotZero": true,
	"Panics": true, "Regexp": true, "Subset": true,
	"True": true, "Zero": true,
}

var pollScopeMethodNames = map[string]bool{
	"Errorf":  true,
	"Fatal":   true,
	"Fatalf":  true,
	"FailNow": true,
}

func checkPollScope(pass *analysis.Pass, insp *inspector.Inspector, cl *claims) {
	insp.Preorder([]ast.Node{(*ast.CallExpr)(nil)}, func(n ast.Node) {
		call := n.(*ast.CallExpr)

		// A polling context needs both signals: an Eventually/Consistently
		// callee shape (case-insensitive so wrappers and re-exports stay
		// covered, package-agnostic by design) and a callback with a typed
		// *gotest.R parameter (so foreign lookalike R types and other
		// func(*R)-taking harnesses like Record stay excluded).
		fnName := calleeName(call.Fun)
		if !strings.EqualFold(fnName, "Eventually") && !strings.EqualFold(fnName, "Consistently") {
			return
		}
		pollParam, funcLit := extractPollCallback(pass, call)
		if funcLit == nil {
			return
		}

		ast.Inspect(funcLit.Body, func(n ast.Node) bool {
			// A nested poll callback owns its subtree; it is visited by its
			// own enclosing call.
			if lit, ok := n.(*ast.FuncLit); ok && lit != funcLit && hasTypedRParam(pass, lit) {
				return false
			}
			innerCall, ok := n.(*ast.CallExpr)
			if !ok {
				return true
			}

			// Case 1: assertion with wrong first arg — gotest.Equal(t, ...) or a
			// foreign library's Equal(t, ...): integrity coverage is deliberately
			// package-agnostic for assertion-shaped names.
			name := assertionFuncName(pass, innerCall.Fun)
			if name == "" && foreignAssertionNames[calleeName(innerCall.Fun)] {
				name = calleeName(innerCall.Fun)
			}
			if name != "" && len(innerCall.Args) > 0 {
				if ident, ok := innerCall.Args[0].(*ast.Ident); ok && ident.Name != pollParam {
					cl.add(PollScope, innerCall.Pos())
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
			onTestT := namedPtrType(pass.TypesInfo.TypeOf(ident), "testing", "T") ||
				namedPtrType(pass.TypesInfo.TypeOf(ident), gotestImportPath, "T")
			if pollScopeMethodNames[sel.Sel.Name] && onTestT && ident.Name != pollParam {
				cl.add(PollScope, innerCall.Pos())
				report(pass, PollScope, ident.Pos(),
					"%s.%s in poll callback bypasses assertion recording — use %s",
					ident.Name, sel.Sel.Name, pollParam)
			}
			return true
		})
	})
}

func hasTypedRParam(pass *analysis.Pass, lit *ast.FuncLit) bool {
	if lit.Type.Params == nil || len(lit.Type.Params.List) != 1 {
		return false
	}
	return namedPtrType(pass.TypesInfo.TypeOf(lit.Type.Params.List[0].Type), gotestImportPath, "R")
}

func extractPollCallback(pass *analysis.Pass, call *ast.CallExpr) (string, *ast.FuncLit) {
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
	if !hasTypedRParam(pass, funcLit) {
		return "", nil
	}
	param := funcLit.Type.Params.List[0]
	if len(param.Names) == 0 {
		return "", nil
	}
	return param.Names[0].Name, funcLit
}

// calleeName returns the last identifier of a call target, for matching and
// messages that should be independent of qualification.
func calleeName(fun ast.Expr) string {
	switch fn := fun.(type) {
	case *ast.SelectorExpr:
		return fn.Sel.Name
	case *ast.Ident:
		return fn.Name
	case *ast.IndexExpr:
		return calleeName(fn.X)
	case *ast.IndexListExpr:
		return calleeName(fn.X)
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
