package main

import (
	"io"
	"time"

	"github.com/mvrahden/go-test/internal/config"
	"github.com/mvrahden/go-test/internal/lint"
)

// Test-only exports, one block per source file.

// args.go
var (
	ExportHasFlag          = hasFlag
	ExportKnownSubcommands = knownSubcommands
)

// bench.go
var ExportRunBench = runBench

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
)

// flags.go
var (
	ExportGotestFlags    = gotestFlags
	ExportTestAllowed    = testAllowed
	ExportSpecAllowed    = specAllowed
	ExportWatchAllowed   = watchAllowed
	ExportSummaryAllowed = summaryAllowed
	ExportFuzzAllowed    = fuzzAllowed
	ExportBenchAllowed   = benchAllowed
	ExportMigrateAllowed = migrateAllowed
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

func ExportSpliceExpr(a corpusArg) string { return a.spliceExpr() }

// lint.go
var ExportLintGitHubArmed = lintGitHubArmed

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

// scaffold.go
var ExportRunScaffold = runScaffold

// spec.go, staticspec.go
var (
	ExportRunSpec          = runSpec
	ExportRunSpecFromInput = runSpecFromInput
	ExportRunStaticSpec    = runStaticSpec
)

// summary.go
var (
	ExportRunSummary          = runSummary
	ExportRunSummaryFromInput = runSummaryFromInput
)

// watch.go
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
