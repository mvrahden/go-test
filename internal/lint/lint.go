package lint

import (
	"bytes"
	"errors"
	"fmt"
	"go/ast"
	"go/token"
	"go/types"
	"maps"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"

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
	// BehaviorWording is expressiveness: a When or It description that opens
	// with the word the spec renders for it says it twice.
	BehaviorWording Rule = "behavior-wording"
	// ConfigNoDeadline is expressiveness: a negative duration disables a
	// deadline, and the name says so where the number does not.
	ConfigNoDeadline Rule = "config-no-deadline"
	BenchLoop        Rule = "bench-loop"
	BenchFixtureIO   Rule = "bench-fixture-io"
	BenchWait        Rule = "bench-wait"
	FuzzDeterminism  Rule = "fuzz-determinism"
	FuzzNoOracle     Rule = "fuzz-no-oracle"
	FuzzSeed         Rule = "fuzz-seed"
	FuzzStructCorpus Rule = "fuzz-struct-corpus"
	FuzzHookIO       Rule = "fuzz-hook-io"
	FuzzRawSeed      Rule = "fuzz-raw-seed"
	// SharedFixtureUndeclared is integrity: window scheduling starts only
	// the fixtures scheduled suites declare, so an undeclared read may hit
	// a fixture that never started or is already released.
	SharedFixtureUndeclared Rule = "shared-fixture-undeclared"
	// SuiteDeclaration, NoEmbedding and FixtureHookCall are integrity: the
	// generator refuses the first two shapes, and a hook called by hand runs
	// twice.
	SuiteDeclaration Rule = "suite-declaration"
	NoEmbedding      Rule = "no-embedding"
	FixtureHookCall  Rule = "fixture-hook-call"
	// GenerationRejects is integrity: a package the generator rejects does
	// not run at all.
	GenerationRejects Rule = "generation-rejects"
	// NolintUnknown is integrity: a nolint comment naming a rule that does
	// not exist suppresses nothing, whatever it was meant to.
	NolintUnknown Rule = "nolint-unknown"
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

// ruleInfo is a rule as the registry states it. Help is its description as
// gotest help lint wraps it; joined, it is the one-line description.
type ruleInfo struct {
	Tier    Tier
	Scope   Scope
	Fixable bool // the rule can attach a suggested fix
	Help    []string
}

// registry lists every rule once, in the order the help prints them.
var registry = []struct {
	Rule Rule
	ruleInfo
}{
	{Focus, ruleInfo{TierIntegrity, ScopeSuites, true, []string{"Focused (F_) suites/methods should not be committed"}}},
	{Receiver, ruleInfo{TierIntegrity, ScopeSuites, true, []string{"Suite methods should use pointer receivers"}}},
	{LifecycleTypo, ruleInfo{TierIntegrity, ScopeSuites, false, []string{"Methods similar to lifecycle hooks (likely typos)"}}},
	{LifecyclePair, ruleInfo{TierIntegrity, ScopeSuites, false, []string{"BeforeAll without matching AfterAll (resource leak)"}}},
	{GeneratedFile, ruleInfo{TierIntegrity, ScopeEverywhere, false, []string{"Generated overlay files should not be in version control"}}},
	{StdlibTest, ruleInfo{TierMigration, ScopeEverywhere, false, []string{"Stdlib test functions — consider using gotest suites"}}},
	{Testify, ruleInfo{TierMigration, ScopeEverywhere, false, []string{"testify imports — consider migrating to gotest"}}},
	{PollScope, ruleInfo{TierIntegrity, ScopePollCallbacks, false, []string{"Outer t used inside Eventually/Consistently callbacks"}}},
	{TestSignature, ruleInfo{TierIntegrity, ScopeSuites, false, []string{"Test methods not accepting *gotest.T or *testing.T"}}},
	{XLifecycle, ruleInfo{TierIntegrity, ScopeSuites, false, []string{"X_ prefix on a lifecycle hook (a no-op)"}}},
	{SuiteLifecycle, ruleInfo{TierIntegrity, ScopeSuites, false, []string{
		"Cleanup/Parallel/Run via t.T(), Cleanup via b.B() or f.F() —",
		"bypass the suite lifecycle",
	}}},
	{SharedFixtureUndeclared, ruleInfo{TierIntegrity, ScopeSuites, false, []string{
		"Reads of a shared fixture the suite never declared as a",
		"field (only declared fixtures are started)",
	}}},
	{GenerationRejects, ruleInfo{TierIntegrity, ScopeSuites, false, []string{"Shapes the generator rejects, where and as it rejects them"}}},
	{NolintUnknown, ruleInfo{TierIntegrity, ScopeEverywhere, false, []string{"//nolint ids that name no gotest rule or golangci-lint linter"}}},
	{SuiteDeclaration, ruleInfo{TierIntegrity, ScopeSuites, true, []string{"Suites/fixtures declared inside a type ( … ) block"}}},
	{NoEmbedding, ruleInfo{TierIntegrity, ScopeSuites, false, []string{
		"Embedded suites, fixtures or hook-carrying types, and",
		"suite-typed fields of a suite or fixture",
	}}},
	{FixtureHookCall, ruleInfo{TierIntegrity, ScopeSuites, false, []string{"Direct calls of a lifecycle hook the harness calls"}}},
	{AssertionSimplify, ruleInfo{TierExpressiveness, ScopeGotestFiles, true, []string{"Simplifiable assertions (True(t, a == b) → Equal, …)"}}},
	{AssertionTypeGuard, ruleInfo{TierIntegrity, ScopeGotestFiles, false, []string{"Nil/Empty on types their runtime guards reject"}}},
	{AssertionRedundant, ruleInfo{TierExpressiveness, ScopeGotestFiles, true, []string{"Assertions made redundant by the following assertion"}}},
	{FailGuard, ruleInfo{TierExpressiveness, ScopeGotestFiles, true, []string{"if cond { Fail/Fatal(...) } guards — use assertions directly"}}},
	{TEscape, ruleInfo{TierExpressiveness, ScopeSuites, true, []string{
		"Unnecessary t.T(), b.B() and f.F() convenience escapes",
		"(incl. Helper/Fatal/Log)",
	}}},
	{BehaviorWording, ruleInfo{TierExpressiveness, ScopeGotestFiles, true, []string{"When(\"when …\") / It(\"it …\") — the spec supplies those words"}}},
	// The rewrite keeps the meaning either way, so config-no-deadline is a
	// matter of saying it in a word: skippable.
	{ConfigNoDeadline, ruleInfo{TierExpressiveness, ScopeEverywhere, true, []string{
		"A negative SuiteConfig/FixtureConfig timeout — every",
		"negative disables the deadline; say so by name",
	}}},
	// A benchmark that never iterates measures nothing — its numbers lie,
	// so bench-loop is integrity. bench-fixture-io is a heuristic about
	// what the timed loop includes; legitimate setups exist, so it stays
	// skippable. bench-wait sits with bench-fixture-io: a deliberate settle
	// inside the loop can be legitimate, so it stays skippable.
	{BenchLoop, ruleInfo{TierIntegrity, ScopeSuites, false, []string{
		"Benchmark methods that never call b.Loop()/b.N (nothing",
		"iterates, so the numbers lie)",
	}}},
	{BenchFixtureIO, ruleInfo{TierExpressiveness, ScopeSuites, false, []string{
		"Fixture-backed reads inside the measured loop (times the",
		"fixture, not the code)",
	}}},
	{BenchWait, ruleInfo{TierExpressiveness, ScopeSuites, false, []string{
		"time.Sleep/Eventually/Consistently inside the measured",
		"loop (times the wait, not the code)",
	}}},
	// fuzz-determinism is integrity: a target reading the clock/RNG/env
	// breaks the replayability the corpus depends on — its outcomes lie.
	// fuzz-struct-corpus is integrity for the same reason from the other
	// side: entries bound to a type's field order silently become different
	// tests when two same-kind fields swap; the legitimate transient state
	// (crasher found, not yet promoted) is suppressible per line.
	// fuzz-no-oracle and fuzz-seed are guidance (crash-only fuzzing and
	// harvester-seeded targets are legitimate), and fuzz-hook-io and
	// fuzz-raw-seed are heuristics with legitimate exceptions (a cheap
	// file read; a seed deliberately left for a signature about to change)
	// — all four stay skippable.
	{FuzzDeterminism, ruleInfo{TierIntegrity, ScopeSuites, false, []string{"Fuzz targets reading time.Now/math-rand/os.Getenv"}}},
	{FuzzNoOracle, ruleInfo{TierExpressiveness, ScopeSuites, false, []string{"Fuzz callbacks that assert nothing (panic-only)"}}},
	{FuzzSeed, ruleInfo{TierExpressiveness, ScopeSuites, false, []string{"Fuzz targets with no f.Add seeds"}}},
	{FuzzStructCorpus, ruleInfo{TierIntegrity, ScopeSuites, false, []string{
		"On-disk corpus entries for struct-typed targets",
		"(field-order-bound — delete them or write typed seeds)",
	}}},
	{FuzzHookIO, ruleInfo{TierExpressiveness, ScopeSuites, false, []string{
		"IO in BeforeEach/AfterEach of fuzzing suites",
		"(hooks replay around every execution)",
	}}},
	{FuzzRawSeed, ruleInfo{TierExpressiveness, ScopeSuites, false, []string{"Raw []byte seeds on positions not taking []byte"}}},
}

// ruleMeta indexes the registry by rule.
var ruleMeta = func() map[Rule]ruleInfo {
	m := make(map[Rule]ruleInfo, len(registry))
	for _, r := range registry {
		m[r.Rule] = r.ruleInfo
	}
	return m
}()

// Known reports whether the rule ID exists.
func Known(r Rule) bool {
	_, ok := ruleMeta[r]
	return ok
}

// RuleIDs returns every registered rule ID, sorted — the registry surface
// drift guards compare against documentation.
func RuleIDs() []Rule {
	ids := make([]Rule, 0, len(ruleMeta))
	for r := range ruleMeta {
		ids = append(ids, r)
	}
	slices.Sort(ids)
	return ids
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

func run(pass *analysis.Pass) (any, error) {
	insp := pass.ResultOf[inspect.Analyzer].(*inspector.Inspector)

	suites := discoverSuites(pass, insp)
	if len(suites) > 0 {
		checkMethods(pass, insp, suites)
		checkFocusPrefixes(pass, suites)
		checkLifecyclePairs(pass, suites)
		checkSharedFixtureUndeclared(pass, insp, suites)
	}

	reached := reachedFixtures(pass)
	checkSuiteDeclaration(pass, insp, reached)
	checkNoEmbedding(pass, insp, reached)
	checkFixtureHookCall(pass, insp)

	checkOrphanedFiles(pass)
	checkNolintIDs(pass)
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
	checkBehaviorWording(pass, insp)
	checkConfigNoDeadline(pass, insp)
	checkBenchLoop(pass, insp, suites)
	checkBenchFixtureIO(pass, insp, suites)
	checkBenchWait(pass, insp, suites)
	checkFuzzDeterminism(pass, insp, suites)
	checkFuzzNoOracle(pass, insp, suites)
	checkFuzzSeed(pass, insp, suites)
	checkFuzzStructCorpus(pass, insp, suites)
	checkFuzzHookIO(pass, insp, suites)
	checkFuzzRawSeed(pass, insp, suites)
	// Last: it stands down on the lines other rules reported.
	checkGenerationRejects(pass)

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

// report reports a finding over rng, the code it is about: a call, an
// expression, a declaration's header.
func report(pass *analysis.Pass, rule Rule, rng analysis.Range, format string, args ...any) {
	reportWithFix(pass, rule, rng, nil, format, args...)
}

func reportWithFix(pass *analysis.Pass, rule Rule, rng analysis.Range, fixes []analysis.SuggestedFix, format string, args ...any) {
	reportedLine(pass, rule, rng.Pos())
	if skipped(rule) {
		return
	}
	if !cfg.disableNolint && isSuppressed(pass, rng.Pos(), rule) {
		return
	}
	pass.Report(analysis.Diagnostic{
		Pos:            rng.Pos(),
		End:            rng.End(),
		Category:       string(rule),
		Message:        fmt.Sprintf(format, args...),
		SuggestedFixes: fixes,
	})
}

// span is a range of source no single node covers.
type span struct{ pos, end token.Pos }

func (s span) Pos() token.Pos { return s.pos }
func (s span) End() token.Pos { return s.end }

// funcHeader is a function's declaration up to its body: what a finding
// about the whole function covers, rather than every line of it.
func funcHeader(fd *ast.FuncDecl) span { return span{fd.Pos(), fd.Type.End()} }

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
	return Suppressed(pass.Fset, pass.Files, pos, rule)
}

// Suppressed reports whether a nolint comment covers rule at pos: on the
// package clause, on the line itself, or in a comment block of its own
// directly above.
func Suppressed(fset *token.FileSet, files []*ast.File, pos token.Pos, rule Rule) bool {
	var file *ast.File
	for _, f := range files {
		if pos >= f.FileStart && pos <= f.FileEnd {
			file = f
			break
		}
	}
	if file == nil {
		return false
	}
	line := fset.Position(pos).Line
	pkgLine := fset.Position(file.Package).Line
	for _, cg := range file.Comments {
		for _, c := range cg.List {
			rules, ok := parseNolint(c.Text)
			if !ok {
				continue
			}
			if !ruleMatched(rules, rule) {
				continue
			}
			cLine := fset.Position(c.Pos()).Line
			if cLine == pkgLine || cLine == line {
				return true
			}
			if fset.Position(cg.End()).Line == line-1 && startsItsLine(fset, file, cg) {
				return true
			}
		}
	}
	return false
}

// startsItsLine reports whether no code precedes the comment group on its
// opening line — a comment trailing the previous statement must not suppress
// the line below it.
func startsItsLine(fset *token.FileSet, file *ast.File, cg *ast.CommentGroup) bool {
	cgLine := fset.Position(cg.Pos()).Line
	standalone := true
	ast.Inspect(file, func(n ast.Node) bool {
		if n == nil || !standalone {
			return false
		}
		if n.End() <= cg.Pos() && fset.Position(n.End()).Line == cgLine {
			standalone = false
			return false
		}
		return n.Pos() < cg.Pos()
	})
	return standalone
}

func parseNolint(text string) (rules map[Rule]bool, ok bool) {
	ids, ok := nolintIDs(text)
	if !ok || len(ids) == 0 || slices.Contains(ids, "all") {
		return nil, ok
	}
	rules = make(map[Rule]bool, len(ids))
	for _, id := range ids {
		rules[Rule(id)] = true
	}
	return rules, true
}

// nolintIDs returns the ids a nolint comment names; none means every rule.
// A reason may follow after " //", with or without ids.
func nolintIDs(text string) (ids []string, ok bool) {
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
		if reason := strings.TrimLeft(rest, " \t"); reason != rest && strings.HasPrefix(reason, "//") {
			return nil, true
		}
		return nil, false
	}
	rest = rest[1:]
	if idx := strings.Index(rest, " //"); idx >= 0 {
		rest = rest[:idx]
	}
	for _, id := range strings.Split(rest, ",") {
		if id = strings.TrimSpace(id); id != "" {
			ids = append(ids, id)
		}
	}
	return ids, true
}

type suiteInfo struct {
	name          string
	pos           token.Pos
	end           token.Pos // of the suite's type name
	methods       map[string]token.Pos
	fixtureFields map[string]bool // names of *...Fixture / *...SharedFixture fields (see bench-fixture-io)
}

func discoverSuites(pass *analysis.Pass, insp *inspector.Inspector) map[string]*suiteInfo {
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
			if strings.HasSuffix(stripped, protocol.SuffixTestSuite) && !isTestifySuite(pass, name) {
				suites[name] = &suiteInfo{
					name:          name,
					pos:           ts.Pos(),
					end:           ts.Name.End(),
					methods:       make(map[string]token.Pos),
					fixtureFields: structFixtureFieldNames(ts.Type),
				}
			}
		}
	})

	return suites
}

// isTestifySuite reports whether the package type name is a testify suite.
func isTestifySuite(pass *analysis.Pass, name string) bool {
	tn, ok := pass.Pkg.Scope().Lookup(name).(*types.TypeName)
	return ok && gotestast.IsTestifySuite(tn.Type())
}

// X_ (skip) prefixes are intentionally not flagged: a skipped test is visibly
// absent from results, whereas a focused test silently hides all other tests
// behind a green CI run.
func checkFocusPrefixes(pass *analysis.Pass, suites map[string]*suiteInfo) {
	for name, s := range suites {
		if strings.HasPrefix(name, protocol.PrefixFocused) {
			stripped := strings.TrimPrefix(name, protocol.PrefixFocused)
			reportWithFix(pass, Focus, span{s.pos, s.end},
				[]analysis.SuggestedFix{{
					Message:   fmt.Sprintf("rename %s to %s", name, stripped),
					TextEdits: unfocusEdits(pass, s.pos),
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

		if !isPointerReceiver(fd.Recv) {
			reportWithFix(pass, Receiver, funcHeader(fd),
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
		// A focused method of any kind focuses the run.
		if strings.HasPrefix(methodName, protocol.PrefixFocused) && isCaseName(stripped) {
			reportWithFix(pass, Focus, funcHeader(fd),
				[]analysis.SuggestedFix{{
					Message:   fmt.Sprintf("rename %s to %s", methodName, stripped),
					TextEdits: unfocusEdits(pass, fd.Name.Pos()),
				}},
				"focused method %s.%s should not be committed", recvName, methodName)
		}
		if strings.HasPrefix(stripped, "Test") && gotestast.IsGoTestName(stripped) {
			if !hasValidTestSignature(pass, fd) {
				report(pass, TestSignature, funcHeader(fd), "test method %s.%s has wrong signature — must accept *gotest.T (or *testing.T)", recvName, methodName)
			}
			return
		}

		if isLifecycleHook(stripped) {
			if strings.HasPrefix(methodName, protocol.PrefixExcluded) {
				report(pass, XLifecycle, funcHeader(fd), "%s", gotestast.ExcludedHookMessage(recvName, methodName))
			}
			return
		}

		if hook, ok := gotestast.MisspelledHook(stripped); ok {
			report(pass, LifecycleTypo, funcHeader(fd), "%s", gotestast.MisspelledHookMessage(recvName, methodName, hook))
		}
	})
}

// isCaseName reports whether name, its prefixes stripped, names a Test,
// Benchmark or Fuzz case as go test would.
func isCaseName(name string) bool {
	for _, prefix := range []string{"Test", protocol.PrefixBenchmark, protocol.PrefixFuzz} {
		if strings.HasPrefix(name, prefix) {
			return gotestast.IsGoTestName(name)
		}
	}
	return false
}

// Only the All pair is checked: BeforeAll holds shared resources for the
// entire suite lifetime, so a missing AfterAll is a likely leak.  BeforeEach
// resources are scoped to a single test and cleaned up with the test.
func checkLifecyclePairs(pass *analysis.Pass, suites map[string]*suiteInfo) {
	for _, s := range suites {
		_, hasBeforeAll := s.methods["BeforeAll"]
		_, hasAfterAll := s.methods["AfterAll"]
		if hasBeforeAll && !hasAfterAll {
			report(pass, LifecyclePair, span{s.pos, s.end}, "suite %s has BeforeAll but no AfterAll — resources may leak", s.name)
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

// escapeConfigs maps a wrapper's letter (T, B, F) to the stdlib methods
// whose call through its escape the rules flag.
var escapeConfigs = map[string]map[string]escapeConfig{
	"T": withEscapes(wrapperEscapeConfigs("T"), map[string]escapeConfig{
		"Name":     availableOn("T", "Name"),
		"Context":  {TEscape, "Context is available on gotest.T — t.T().Context() drops the suite's Timeout", false, true, false},
		"Deadline": {TEscape, "use t.Context().Deadline() — t.T().Deadline() reports go test's -timeout, not the suite's Timeout", false, true, false},
		"Parallel": {SuiteLifecycle, "use SuiteConfig.Parallel instead — T.Parallel bypasses suite lifecycle coordination", true, false, false},
		"Run":      {SuiteLifecycle, "use It or When instead — T.Run bypasses gotest wrapping", false, false, false},
	}),
	"B": withEscapes(wrapperEscapeConfigs("B"), map[string]escapeConfig{
		"Loop":         availableOn("B", "Loop"),
		"Name":         availableOn("B", "Name"),
		"ReportAllocs": availableOn("B", "ReportAllocs"),
		"SetBytes":     availableOn("B", "SetBytes"),
		"ReportMetric": availableOn("B", "ReportMetric"),
	}),
	"F": withEscapes(wrapperEscapeConfigs("F"), map[string]escapeConfig{
		"Add":  {TEscape, "Add is available on gotest.F — f.F().Add skips the seed buffer: a struct seed panics inside testing, and the Add-after-Fuzz check is bypassed", false, true, false},
		"Name": availableOn("F", "Name"),
	}),
}

// wrapperEscapeConfigs is the table every wrapper shares; the texts that
// name a T method read the same on each.
func wrapperEscapeConfigs(wrapper string) map[string]escapeConfig {
	return map[string]escapeConfig{
		"Errorf":  availableOn(wrapper, "Errorf"),
		"FailNow": availableOn(wrapper, "FailNow"),
		"Skipf":   availableOn(wrapper, "Skipf"),
		"Context": availableOn(wrapper, "Context"),
		"Setenv":  availableOn(wrapper, "Setenv"),
		"TempDir": availableOn(wrapper, "TempDir"),
		"Skip":    {TEscape, fmt.Sprintf("must use Skipf instead — unnecessary %s escape", wrapper), false, false, false},
		"SkipNow": {TEscape, fmt.Sprintf("must use Skipf instead — unnecessary %s escape", wrapper), false, false, false},
		"Cleanup": {SuiteLifecycle, "use AfterEach or AfterAll for cleanup — T.Cleanup bypasses suite lifecycle", true, false, false},
		"Helper":  {TEscape, "never call Helper — gotest resolves call sites automatically; Helper degrades failure locations", false, false, true},
		"Log":     {TEscape, "use assertion message args instead — T.Log bypasses the failure report", false, false, true},
		"Fatal":   {TEscape, "use assertions instead — T.Fatal bypasses the assertion tracer", false, false, true},
		"Fatalf":  {TEscape, "use assertions instead — T.Fatalf bypasses the assertion tracer", false, false, true},
		"Error":   {TEscape, "use assertions instead — T.Error bypasses the assertion tracer", false, false, true},
		"Fail":    {TEscape, "use assertions instead — T.Fail bypasses the assertion tracer", false, false, true},
		"Logf":    {TEscape, "use assertion message args instead — T.Logf bypasses the failure report", false, false, true},
	}
}

// availableOn flags a method the wrapper offers itself, fixed by calling it there.
func availableOn(wrapper, method string) escapeConfig {
	return escapeConfig{TEscape, fmt.Sprintf("%s is available on gotest.%s — unnecessary %s escape", method, wrapper, wrapper), false, true, false}
}

// withEscapes adds extras to table, an extra replacing a shared entry.
func withEscapes(table, extras map[string]escapeConfig) map[string]escapeConfig {
	maps.Copy(table, extras)
	return table
}

// escapeFixTargets names the gotest.T call an autofix writes when it is not
// the escaped method itself.
var escapeFixTargets = map[string]string{
	"Deadline": "Context().Deadline",
}

type deferredReport struct {
	rule     Rule
	pos, end token.Pos
	message  string
}

func checkTEscape(pass *analysis.Pass, insp *inspector.Inspector, suites map[string]*suiteInfo, cl *claims) {
	mr := buildMethodReach(pass, insp, 5)
	reported := map[token.Pos]bool{}
	var deferred []deferredReport

	scanBody := func(body *ast.BlockStmt, isSuiteMethod bool, gotestTVars map[string]string) {
		tVars := map[string]string{}
		ast.Inspect(body, func(n ast.Node) bool {
			switch node := n.(type) {
			case *ast.AssignStmt:
				trackTVarAssign(pass.TypesInfo, node, tVars, gotestTVars)
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

		gotestTVars := map[string]string{}
		if isSuiteMethod && fd.Type.Params != nil {
			for _, field := range fd.Type.Params.List {
				if w := gotestWrapperOf(pass, field); w != "" {
					for _, name := range field.Names {
						gotestTVars[name.Name] = w
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
						scanBody(lit.Body, false, map[string]string{})
					}
				}
			}
		}
	}

	for _, d := range deferred {
		if !reported[d.pos] {
			reported[d.pos] = true
			cl.add(d.rule, d.pos)
			report(pass, d.rule, span{d.pos, d.end}, "%s", d.message)
		}
	}
}

func trackTVarAssign(info *types.Info, assign *ast.AssignStmt, tVars, gotestTVars map[string]string) {
	for i, rhs := range assign.Rhs {
		if i >= len(assign.Lhs) {
			break
		}
		lhsId, ok := assign.Lhs[i].(*ast.Ident)
		if !ok {
			continue
		}
		if w, ok := wrapperEscape(info, rhs); ok {
			tVars[lhsId.Name] = w
			continue
		}
		if id, ok := rhs.(*ast.Ident); ok {
			if w := tVars[id.Name]; w != "" {
				tVars[lhsId.Name] = w
			}
			if w := gotestTVars[id.Name]; w != "" {
				gotestTVars[lhsId.Name] = w
			}
		}
	}
}

func reportDirectEscape(pass *analysis.Pass, cl *claims, call *ast.CallExpr, isSuiteMethod bool, tVars map[string]string, reported map[token.Pos]bool) {
	sel, _ := call.Fun.(*ast.SelectorExpr)
	if sel == nil {
		return
	}

	wrapper, isDirect := wrapperEscape(pass.TypesInfo, sel.X)
	if !isDirect {
		if id, ok := sel.X.(*ast.Ident); ok {
			wrapper = tVars[id.Name]
		}
	}
	if esc, ok := escapeConfigs[wrapper][sel.Sel.Name]; ok && (!esc.suiteOnly || isSuiteMethod) {
		reported[call.Pos()] = true
		cl.add(esc.rule, call.Pos())
		if esc.canAutofix && isDirect {
			inner := sel.X.(*ast.CallExpr)
			innerSel := inner.Fun.(*ast.SelectorExpr)
			target := sel.Sel.Name
			if to, ok := escapeFixTargets[target]; ok {
				target = to
			}
			reportWithFix(pass, esc.rule, call,
				[]analysis.SuggestedFix{{
					Message: fmt.Sprintf("call %s directly", target),
					TextEdits: []analysis.TextEdit{{
						Pos:     innerSel.X.End(),
						End:     sel.Sel.End(),
						NewText: []byte("." + target),
					}},
				}},
				"%s", esc.message)
		} else {
			report(pass, esc.rule, call, "%s", esc.message)
		}
		return
	}

	if assertionFuncName(pass, call.Fun) != "" && len(call.Args) > 0 {
		arg := call.Args[0]
		if w, ok := wrapperEscape(pass.TypesInfo, arg); ok {
			inner := arg.(*ast.CallExpr)
			reported[inner.Pos()] = true
			innerSel := inner.Fun.(*ast.SelectorExpr)
			reportWithFix(pass, TEscape, inner,
				[]analysis.SuggestedFix{{
					Message: fmt.Sprintf("pass gotest.%s directly", w),
					TextEdits: []analysis.TextEdit{{
						Pos:     innerSel.X.End(),
						End:     inner.End(),
						NewText: []byte(""),
					}},
				}},
				"pass gotest.%s directly to %s — unnecessary %s escape", w, sel.Sel.Name, w)
			return
		}
		if id, ok := arg.(*ast.Ident); ok && tVars[id.Name] != "" {
			w := tVars[id.Name]
			reported[arg.Pos()] = true
			report(pass, TEscape, arg,
				"pass gotest.%s directly to %s — unnecessary %s escape", w, sel.Sel.Name, w)
			return
		}
	}
}

func collectInterproceduralEscape(call *ast.CallExpr, tVars, gotestTVars map[string]string, mr *methodReach, results *[]deferredReport) {
	methods := mr.reachedMethods(call, tVars, gotestTVars)
	for key, positions := range methods {
		wrapper, method, _ := strings.Cut(key, ".")
		esc := escapeConfigs[wrapper][method]
		for pos := range positions {
			*results = append(*results, deferredReport{esc.rule, pos, mr.ends[pos], esc.message})
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

// gotestWrapperOf names the gotest wrapper a parameter field is typed as:
// "T", "B", "F", or "" for any other type.
func gotestWrapperOf(pass *analysis.Pass, field *ast.Field) string {
	t := pass.TypesInfo.TypeOf(field.Type)
	for _, w := range []string{"T", "B", "F"} {
		if namedPtrType(t, gotestImportPath, w) {
			return w
		}
	}
	return ""
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
	// ends maps a marked call's position to its end.
	ends map[token.Pos]token.Pos
}

func buildMethodReach(pass *analysis.Pass, insp *inspector.Inspector, maxDepth int) *methodReach {
	mr := &methodReach{
		pass:      pass,
		funcDecls: map[types.Object]*ast.FuncDecl{},
		params:    map[*ast.FuncDecl]map[int]map[string]map[token.Pos]bool{},
		edges:     map[*ast.FuncDecl][]callEdge{},
		ends:      map[token.Pos]token.Pos{},
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

// markEscape marks call, a method on the parameter's stdlib value, under
// "<wrapper>.<method>" when the rules flag it beyond direct calls.
func (mr *methodReach) markEscape(fd *ast.FuncDecl, paramIdx int, wrapper string, call *ast.CallExpr) {
	method := call.Fun.(*ast.SelectorExpr).Sel.Name
	if esc, ok := escapeConfigs[wrapper][method]; !ok || esc.directOnly {
		return
	}
	mr.mark(fd, paramIdx, wrapper+"."+method, call.Pos())
	mr.ends[call.Pos()] = call.End()
}

// stdlibWrapper names the wrapper whose stdlib value id denotes: "T", "B" or
// "F" for a *testing.T, *testing.B or *testing.F, "" otherwise. An ident the
// type checker could not resolve counts as a *testing.T, conservatively.
func (mr *methodReach) stdlibWrapper(id *ast.Ident) string {
	obj := mr.pass.TypesInfo.Uses[id]
	if obj == nil {
		return "T"
	}
	for _, w := range []string{"T", "B", "F"} {
		if namedPtrType(obj.Type(), "testing", w) {
			return w
		}
	}
	return ""
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
			trackParamFlow(mr.pass.TypesInfo, node, aliases)
		case *ast.CallExpr:
			sel, ok := node.Fun.(*ast.SelectorExpr)
			if ok {
				// A method called on the parameter itself is an escape only
				// when the parameter is the stdlib value: Errorf on a
				// *gotest.T helper parameter is the call the rule recommends.
				if id, ok := sel.X.(*ast.Ident); ok {
					if idx, ok := aliases[id.Name]; ok {
						mr.markEscape(fd, idx, mr.stdlibWrapper(id), node)
					}
				}
				if w, ok := wrapperEscape(mr.pass.TypesInfo, sel.X); ok {
					innerSel := sel.X.(*ast.CallExpr).Fun.(*ast.SelectorExpr)
					if id, ok := innerSel.X.(*ast.Ident); ok {
						if idx, ok := aliases[id.Name]; ok {
							mr.markEscape(fd, idx, w, node)
						}
					}
				}
			}
			if callee := mr.resolveCallee(node); callee != nil {
				edge := callEdge{callee: callee, args: map[int]int{}}
				for argIdx, arg := range node.Args {
					if idx := exprToParamIdx(mr.pass.TypesInfo, arg, aliases); idx >= 0 {
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

func (mr *methodReach) reachedMethods(call *ast.CallExpr, tVars, gotestTVars map[string]string) map[string]map[token.Pos]bool {
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
		_, tainted := wrapperEscape(mr.pass.TypesInfo, arg)
		if !tainted {
			if id, ok := arg.(*ast.Ident); ok {
				tainted = tVars[id.Name] != "" || gotestTVars[id.Name] != ""
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
func trackParamFlow(info *types.Info, assign *ast.AssignStmt, aliases map[string]int) {
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
		if _, ok := wrapperEscape(info, rhs); ok {
			call := rhs.(*ast.CallExpr)
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
func exprToParamIdx(info *types.Info, expr ast.Expr, aliases map[string]int) int {
	if id, ok := expr.(*ast.Ident); ok {
		if idx, ok := aliases[id.Name]; ok {
			return idx
		}
	}
	if _, ok := wrapperEscape(info, expr); ok {
		call := expr.(*ast.CallExpr)
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

// wrapperEscape reports whether expr calls a gotest wrapper's escape — T() on
// a *gotest.T, B() on a *gotest.B, F() on a *gotest.F — and names the
// wrapper; the same method on any other type is not the escape.
func wrapperEscape(info *types.Info, expr ast.Expr) (wrapper string, ok bool) {
	call, ok := expr.(*ast.CallExpr)
	if !ok || len(call.Args) != 0 {
		return "", false
	}
	sel, ok := call.Fun.(*ast.SelectorExpr)
	if !ok {
		return "", false
	}
	switch w := sel.Sel.Name; w {
	case "T", "B", "F":
		if namedPtrType(info.TypeOf(sel.X), gotestImportPath, w) {
			return w, true
		}
	}
	return "", false
}

func checkOrphanedFiles(pass *analysis.Pass) {
	for _, file := range pass.Files {
		name := filepath.Base(pass.Fset.File(file.Pos()).Name())
		if !about.PSuiteRegex.MatchString(name) {
			continue
		}
		switch tracked, err := gitTracks(pass.Fset.File(file.Pos()).Name()); {
		case err != nil:
			report(pass, GeneratedFile, span{file.Package, file.Name.End()}, "generated file %s may be checked into version control: git cannot tell (%v)", name, err)
		case tracked:
			report(pass, GeneratedFile, span{file.Package, file.Name.End()}, "generated file %s should not be checked into version control", name)
		}
	}
}

// gitTracks reports whether path may be in version control: true when git
// tracks it or when it lies outside a git repository. err says why git could
// not tell, git missing or its repository unreadable.
func gitTracks(path string) (bool, error) {
	cmd := exec.Command("git", "ls-files", "-z", "--", filepath.Base(path)) //nolint:gosec // G204: git with a file the load read
	cmd.Dir = filepath.Dir(path)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	out, err := cmd.Output()
	switch {
	case err == nil:
		return len(out) > 0, nil
	case strings.Contains(stderr.String(), "not a git repository"):
		return true, nil
	}
	if msg := strings.TrimSpace(stderr.String()); msg != "" {
		return false, errors.New(msg)
	}
	return false, err
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
		if !strings.HasPrefix(name, "Test") || !gotestast.IsGoTestName(name) {
			return
		}
		if isGeneratedFile(pass, fd.Pos()) {
			return
		}
		if fd.Type.Params == nil || len(fd.Type.Params.List) != 1 {
			return
		}
		if !isTestingT(pass, fd.Type.Params.List[0].Type) {
			return
		}
		report(pass, StdlibTest, funcHeader(fd), "stdlib test %s — consider using a gotest suite", name)
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
				report(pass, Testify, imp, "testify import %s — consider migrating to gotest", path)
			}
		}
	}
}

func isGeneratedFile(pass *analysis.Pass, pos token.Pos) bool {
	return about.PSuiteRegex.MatchString(filepath.Base(pass.Fset.Position(pos).Filename))
}

func isTestingT(pass *analysis.Pass, expr ast.Expr) bool {
	star, ok := expr.(*ast.StarExpr)
	if !ok {
		return false
	}
	sel, ok := star.X.(*ast.SelectorExpr)
	return ok && sel.Sel.Name == "T" && isPkgRef(pass, sel.X, "testing")
}

func hasValidTestSignature(pass *analysis.Pass, fd *ast.FuncDecl) bool {
	params := fd.Type.Params
	if params == nil || len(params.List) < 1 || len(params.List) > 2 {
		return false
	}
	return isSupportedT(pass, params.List[0].Type)
}

// isSupportedT reports whether expr names *gotest.T or *testing.T, by the
// type behind it: an aliased import spells the same type another way.
func isSupportedT(pass *analysis.Pass, expr ast.Expr) bool {
	ptr, ok := pass.TypesInfo.TypeOf(expr).(*types.Pointer)
	if !ok {
		return false
	}
	named, ok := ptr.Elem().(*types.Named)
	if !ok || named.Obj().Name() != "T" || named.Obj().Pkg() == nil {
		return false
	}
	path := named.Obj().Pkg().Path()
	return path == gotestImportPath || path == "testing"
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

func isLifecycleHook(name string) bool {
	return slices.Contains(gotestast.LifecycleHooks, name)
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
					report(pass, PollScope, ident,
						"use %s instead of %s in poll callback passed to %s",
						pollParam, ident.Name, fnName)
				}
				return true
			}

			// Case 2: the outer t handed to a helper — check(t)
			for _, arg := range innerCall.Args {
				if ident, ok := arg.(*ast.Ident); ok && isTestT(pass, ident) && ident.Name != pollParam {
					cl.add(PollScope, innerCall.Pos())
					report(pass, PollScope, ident,
						"%s passed to %s in poll callback bypasses assertion recording — use %s",
						ident.Name, pick(calleeName(innerCall.Fun) == "", "a function", calleeName(innerCall.Fun)), pollParam)
				}
			}

			// Case 3: direct method call — t.Errorf(...), t.Fatal(...)
			sel, ok := innerCall.Fun.(*ast.SelectorExpr)
			if !ok {
				return true
			}
			ident, ok := sel.X.(*ast.Ident)
			if !ok {
				return true
			}
			if pollScopeMethodNames[sel.Sel.Name] && isTestT(pass, ident) && ident.Name != pollParam {
				cl.add(PollScope, innerCall.Pos())
				report(pass, PollScope, ident,
					"%s.%s in poll callback bypasses assertion recording — use %s",
					ident.Name, sel.Sel.Name, pollParam)
			}
			return true
		})
	})
}

// isTestT reports whether ident is a *testing.T or a *gotest.T.
func isTestT(pass *analysis.Pass, ident *ast.Ident) bool {
	t := pass.TypesInfo.TypeOf(ident)
	return namedPtrType(t, "testing", "T") || namedPtrType(t, gotestImportPath, "T")
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
