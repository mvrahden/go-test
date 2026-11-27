package main

import (
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/mvrahden/go-test/internal/config"
	"github.com/mvrahden/go-test/internal/gotestaudit"
	"github.com/mvrahden/go-test/internal/gotestgen"
	"github.com/mvrahden/go-test/internal/gotestrunner"
	"github.com/mvrahden/go-test/internal/gotestspec"
	"github.com/mvrahden/go-test/internal/lint"
	"github.com/mvrahden/go-test/internal/mutate"
	"github.com/mvrahden/go-test/internal/protocol"
)

// Test-only exports, one block per source file.

// args.go
var (
	ExportHasFlag          = hasFlag
	ExportKnownSubcommands = knownSubcommands
)

// bench.go
var ExportRunBench = runBench

// clean.go
var ExportGeneratedFiles = generatedFiles
var ExportIsGenerated = isGenerated

// cli.go
var (
	ExportResolveGlobalTimeout = resolveGlobalTimeout
	ExportEnsureCoverProfile   = ensureCoverProfile
)

// ownOf parses raw own-flag tokens as every command's flags are parsed, with
// every flag allowed, so the parse helpers can be exercised from tokens.
func ownOf(args []string) (OwnFlags, error) {
	allowed := make(map[string]bool, len(gotestFlags))
	for name := range gotestFlags {
		allowed[name] = true
	}
	own, _, err := SplitArgs(args, allowed)
	return own, err
}

func ExportParseMinFlag(args []string) (int, error) {
	own, err := ownOf(args)
	if err != nil {
		return 0, err
	}
	return parseMinFlag(own)
}

func ExportParseParallelFlag(args []string) (int, error) {
	own, err := ownOf(args)
	if err != nil {
		return 0, err
	}
	return parseParallelFlag(own)
}

func ExportParseCompileParallelFlag(args []string) (int, error) {
	own, err := ownOf(args)
	if err != nil {
		return 0, err
	}
	return parseCompileParallelFlag(own)
}

func ExportParseSetupTimeoutFlag(args []string) (time.Duration, error) {
	own, err := ownOf(args)
	if err != nil {
		return 0, err
	}
	return parseSetupTimeoutFlag(own)
}

func ExportParseGlobalTimeoutFlag(args []string) (time.Duration, error) {
	own, err := ownOf(args)
	if err != nil {
		return 0, err
	}
	return parseGlobalTimeoutFlag(own)
}

func ExportParseExecFlags(ownArgs, goTestArgs []string, projCfg *config.ProjectConfig) (ExecConfig, error) {
	own, err := ownOf(ownArgs)
	if err != nil {
		return ExecConfig{}, err
	}
	return parseExecFlags(own, goTestArgs, projCfg)
}

// color.go
var (
	ExportColorDisabled = colorDisabled
	ExportCheckFormat   = checkFormat
)

var ExportRenderSpec = renderSpec

// ExportSetStdoutIsTerminal swaps the terminal probe and returns its restorer.
func ExportSetStdoutIsTerminal(f func() bool) func() {
	prev := stdoutIsTerminal
	stdoutIsTerminal = f
	return func() { stdoutIsTerminal = prev }
}

// discover.go
type (
	ExportDiscoverOutput  = discoverOutput
	ExportDiscoverPackage = discoverPackage
	ExportDiscoverSuite   = discoverSuite
)

var (
	ExportNewDiscoverOutput  = newDiscoverOutput
	ExportBuildDiscoverSuite = buildDiscoverSuite
	ExportDiscover           = discover
	ExportTwinTestNames      = twinTestNames
)

// flags.go
var (
	ExportGotestFlags      = gotestFlags
	ExportTestAllowed      = testAllowed
	ExportSpecAllowed      = specAllowed
	ExportWatchAllowed     = watchAllowed
	ExportSummaryAllowed   = summaryAllowed
	ExportFuzzAllowed      = fuzzAllowed
	ExportBenchAllowed     = benchAllowed
	ExportMigrateAllowed   = migrateAllowed
	ExportControlAllowed   = controlAllowed
	ExportAuditAllowed     = auditAllowed
	ExportScenariosAllowed = scenariosAllowed
)

// focusguard.go
var ExportDetectCIEnv = detectCIEnv

// fuzz.go
type ExportFuzzSession = fuzzSession

var (
	ExportPlanFuzzSession   = planFuzzSession
	ExportSelectFuzzTargets = selectFuzzTargets
)

func ExportParseForFlag(args []string) (time.Duration, bool, error) {
	own, err := ownOf(args)
	if err != nil {
		return 0, false, err
	}
	return parseForFlag(own)
}

func ExportParseJobsFlag(args []string) (int, error) {
	own, err := ownOf(args)
	if err != nil {
		return 0, err
	}
	return parseJobsFlag(own)
}

// fuzzsummary.go
var (
	ExportFuzzSessionLine           = fuzzSessionLine
	ExportRenderFuzzSessionMarkdown = renderFuzzSessionMarkdown
)

// fuzztriage.go
type ExportCorpusArg = corpusArg

var (
	ExportParseCorpusFile     = parseCorpusFile
	ExportExtractDecodedInput = extractDecodedInput
	ExportExtractCause        = extractCause
	ExportPromoteCrasher      = promoteCrasher
	ExportClassifyRerun       = classifyRerun
)

func ExportSpliceExpr(a corpusArg) (string, error) { return a.spliceExpr() }

// lint.go
var (
	ExportLintGitHubArmed       = lintGitHubArmed
	ExportAppendLintStepSummary = appendLintStepSummary
	ExportRelPath               = relPath
)

// ExportRunLintGitHub runs lint in-process with GitHub annotations.
func ExportRunLintGitHub(stdout, stderr io.Writer, dir string, args []string) (code int, ok bool) {
	return runLintInProcess(stdout, stderr, dir, args, true)
}

// ExportRunLintPlain runs lint in-process with plain-text findings.
func ExportRunLintPlain(stderr io.Writer, dir string, args []string) (code int, ok bool) {
	return runLintInProcess(io.Discard, stderr, dir, args, false)
}

// ExportResetLintSkipFlag restores an analyzer skip flag after a test has
// set it through the GitHub lint mode; the flag set is process-global.
func ExportResetLintSkipFlag(name string) error {
	return lint.Analyzer.Flags.Set(name, "false")
}

// prepare.go
var ExportPrepareHarvestSeeds = prepareHarvestSeeds

// scaffold.go
var ExportRunScaffold = runScaffold

// spec.go, staticspec.go
var (
	ExportRunSpec          = runSpec
	ExportRunSpecFromInput = func(input, format, output string, noColor, renderOnly bool) int {
		return runSpecFromInput(input, format, output, noColor, renderOnly, false, nil, nil)
	}
	ExportRunStaticSpec = runStaticSpec
)

// summary.go
var (
	ExportRunSummary          = runSummary
	ExportRunSummaryFromInput = func(input, format, output, coverageProfile string, noColor, github, renderOnly bool, badgePath string) int {
		return runSummaryFromInput(input, format, output, coverageProfile, noColor, github, renderOnly, false, badgePath)
	}
)

// watch.go
var (
	ExportDirPattern = dirPattern
	ExportRerunDirs  = rerunDirs
	ExportWatchLoop  = watchLoop
)

func ExportParseDebounceFlag(args []string) (time.Duration, error) {
	own, err := ownOf(args)
	if err != nil {
		return 0, err
	}
	return parseDebounceFlag(own)
}

var (
	ExportIsGoFile        = isGoFile
	ExportDirsToPatterns  = dirsToPatterns
	ExportReplacePatterns = replacePatterns
	ExportBenchDeltaLines = benchDeltaLines
	ExportRenderWatchRun  = renderWatchRun
)

// ExportRemoveLintTemp is removeLintTemp with remove standing in for
// os.RemoveAll.
func ExportRemoveLintTemp(dir string, bound time.Duration, remove func(string) error) {
	prev := removeAll
	removeAll = remove
	defer func() { removeAll = prev }()
	removeLintTemp(dir, bound)
}

// signals.go
var (
	ExportForcedStop          = forcedStop
	ExportShutdownContextThen = shutdownContextThen
	ExportDuplicateWindow     = duplicateWindow
)

// ExportObserveExit has the second signal end the process by observe instead
// of os.Exit, until the returned func restores it.
func ExportObserveExit(observe func(code int, hurry <-chan struct{})) (restore func()) {
	exit = observe
	return func() { exit = gotestrunner.Exit }
}

// ExportHoldForcedExit holds forcedExit, as a write that must not stop
// halfway does, until the returned func releases it.
func ExportHoldForcedExit() (release func()) {
	forcedExit.Lock()
	return forcedExit.Unlock
}

// provenance.go
var (
	ExportReadProvenance = readProvenance
	ExportModulePath     = modulePath
)

// the words after -args
var (
	ExportSpecRequested    = specRequested
	ExportStripJSONFlag    = stripJSONFlag
	ExportContainsHelpFlag = containsHelpFlag
)

// cli.go
var ExportCommandHint = commandHint

// focusguard.go
var ExportFocusGate = focusGate

// color.go
var ExportReportColorDisabled = reportColorDisabled

// declarations.go
// ExportDeclarationIndexOf loads patterns as a run loads them and returns
// the declaration index read from them.
func ExportDeclarationIndexOf(patterns []string) (gotestspec.DeclarationIndex, error) {
	pkgs, err := gotestgen.LoadRunPackages(patterns, nil)
	if err != nil {
		return nil, err
	}
	return buildDeclarationIndex(pkgs.Loaded), nil
}

// phases.go
type ExportPhases = phases

// ExportAgentOutput is whether a text run given ownArgs under env prints for
// an agent, and why.
func ExportAgentOutput(ownArgs []string, env map[string]string) (bool, string, error) {
	own, err := ownOf(ownArgs)
	if err != nil {
		return false, "", err
	}
	on, why := agentOutput(own, func(k string) string { return env[k] })
	return on, why, nil
}

var ExportRerunCommand = rerunCommand

// ExportLiveStream is liveStream over the own flags ownArgs, a -json run or
// not, and env.
func ExportLiveStream(ownArgs []string, jsonMode bool, env map[string]string) (bool, error) {
	own, err := ownOf(ownArgs)
	if err != nil {
		return false, err
	}
	return liveStream(own, jsonMode, func(k string) string { return env[k] })
}

// ExportRunsEntryJSON is one run of the runs listing as JSON.
func ExportRunsEntryJSON(k *gotestrunner.KeptRun) string {
	data, _ := json.Marshal(entryOf(k, nil))
	return string(data)
}

// audit.go, auditmutate.go, auditmutate_run.go

// ExportAuditView is what an audit's flags set for --mutate.
type ExportAuditView struct {
	Mutate, Run, Strings bool
	Budget               time.Duration
	Parallel             int
}

// ExportParseAudit reads audit's own flags and the rest as runAudit does.
func ExportParseAudit(args []string) (ExportAuditView, error) {
	own, rest, err := SplitArgs(args, auditAllowed)
	if err != nil {
		return ExportAuditView{}, err
	}
	a, err := parseAuditArgs(own, rest)
	return ExportAuditView{Mutate: a.mutate, Run: a.run, Strings: a.mutateStrings, Budget: a.mutateBudget, Parallel: a.parallel}, err
}

// ExportMutationJobs is how many mutants run at once under --parallel.
func ExportMutationJobs(parallel int) (jobs, per int) {
	return (&auditArgs{parallel: parallel}).mutationJobs()
}

// ExportRunEnv and ExportCoverageEnv are the environments of a run on the
// mutants of the overlay and of the coverage run keeping suites in dir.
func ExportRunEnv(env []string, overlay string, id int) []string {
	return (&mutation{overlay: overlay}).runEnv(env, id)
}

func ExportCoverageEnv(env []string, dir string) []string {
	return (&mutation{dir: dir}).coverageEnv(env)
}

// ExportWriteOverlay writes the mutants' overlay of files in dir and returns
// its replacements.
func ExportWriteOverlay(dir string, files map[string][]byte) (map[string]string, error) {
	m := &mutation{dir: dir, overlay: filepath.Join(dir, "overlay.json"), plan: &mutate.Plan{Files: files}}
	if err := os.MkdirAll(filepath.Join(dir, "src"), 0o700); err != nil {
		return nil, err
	}
	if err := m.writeOverlay(); err != nil {
		return nil, err
	}
	data, err := os.ReadFile(m.overlay)
	if err != nil {
		return nil, err
	}
	var o struct{ Replace map[string]string }
	return o.Replace, json.Unmarshal(data, &o)
}

var ExportTestsFirst = testsFirst

// ExportReadStream writes a -json stream in chunks to the writer a mutant's
// run reads, and returns what it read.
func ExportReadStream(chunks []string, collect bool) (killedBy string, failed []string, took map[string]time.Duration) {
	w := &streamWriter{collect: collect, took: map[string]time.Duration{}}
	if !collect {
		w.first = make(chan string, 1)
	}
	for _, c := range chunks {
		_, _ = w.Write([]byte(c))
	}
	return w.killedBy, w.failed, w.took
}

// ExportLimit is a mutant's first and second limit over suites, given
// their times with none switched on and how long that whole run took.
func ExportLimit(took map[string]time.Duration, whole time.Duration, suites ...string) (first, second time.Duration) {
	j := &mutantJudge{took: took, whole: whole}
	var runs []*suiteRun
	for _, s := range suites {
		pkg, name, _ := strings.Cut(s, " ")
		runs = append(runs, &suiteRun{pkg: pkg, name: name})
	}
	return j.limit(runs), j.secondLimit(runs)
}

// ExportBaseline is what readBaseline made of a coverage run.
type ExportBaseline struct {
	Passed   bool
	Failed   []string
	Suites   []string
	Ran      map[string][]int // per suite, the lines of file it executed
	Binaries int
}

// ExportReadBaseline reads the coverage run kept in dir/suites for the
// mutated pkgs, its executed lines told for file.
func ExportReadBaseline(dir string, pkgs []string, events []gotestspec.TestEvent, exit int, profile, file string) (ExportBaseline, error) {
	b, n, err := (&mutation{dir: dir, pkgs: pkgs}).readBaseline(events, exit, profile)
	if err != nil {
		return ExportBaseline{}, err
	}
	out := ExportBaseline{Passed: b.passed, Failed: b.failed, Binaries: n, Ran: map[string][]int{}}
	for _, s := range b.suites {
		out.Suites = append(out.Suites, s.label())
		for l := 1; l <= 200; l++ {
			if s.executed[file][l] {
				out.Ran[s.label()] = append(out.Ran[s.label()], l)
			}
		}
	}
	return out, nil
}

// ExportMutate prepares the mutants of files in the current directory and
// judges them as audit --mutate does, by a baseline in which suite ran
// every line of them and passed when passed, its runs made by runner.
func ExportMutate(files []gotestaudit.ProdFile, budget time.Duration, suite string, passed bool, runner string) (*gotestaudit.Document, error) {
	a := &auditArgs{mutate: true, run: true, mutateBudget: budget, patterns: []string{"./..."}}
	doc := &gotestaudit.Document{}
	m, err := a.prepareMutation(doc, files)
	if err != nil || m == nil {
		return doc, err
	}
	defer m.release()
	if err := m.testPackages(a); err != nil {
		return doc, err
	}
	pkg, name, _ := strings.Cut(suite, " ")
	s := &suiteRun{pkg: pkg, name: name, executed: mutate.Executed{}}
	for i := range m.all {
		mu := &m.all[i]
		s.executed.Add([]mutate.Block{{File: mutate.ProfileFile(mu), StartLine: mu.Line, EndLine: mu.Line, Ran: true}})
	}
	b := &baseline{passed: passed, suites: []*suiteRun{s}}
	if !passed {
		b.failed = []string{suite}
	}
	defer func(prev func() (string, error)) { mutantRunner = prev }(mutantRunner)
	mutantRunner = func() (string, error) { return runner, nil }
	a.judge(doc, m, b)
	return doc, nil
}

// ExportNarrowedTo is the Changed of a run that runs pkgs.
var ExportNarrowedTo = narrowedTo

// recordstamp.go

// ExportHeldCurrent is whether --changed holds a recorded verdict as current.
var ExportHeldCurrent = heldCurrent

// ExportStampRecord stamps a run's record with its mode and narrowing.
var ExportStampRecord = stampRecord

// changed.go

// ExportNarrowedBy is the narrowing of a run given its flags, groups,
// --failed selection, --changed --failed selection and focus.
func ExportNarrowedBy(args, groups []string, failedOnly, changedFailedOnly map[string]string, focused bool) func(string) bool {
	cfg := &ExecConfig{GoTestArgs: args, Groups: groups, Only: failedOnly}
	return narrowedBy(cfg, &runPlan{only: changedFailedOnly, focused: focused})
}

// runs.go

// ExportRunWord is the word gotest runs prints for a run's result.
func ExportRunWord(exit, packagesPassed int, tests *protocol.Tally) string {
	r := runsResult{ExitCode: exit, Packages: runsTally{Passed: packagesPassed}}
	if tests != nil {
		r.Tests = &runsTally{Passed: tests.Passed, Failed: tests.Failed, Skipped: tests.Skipped}
	}
	return runWord(&r)
}
