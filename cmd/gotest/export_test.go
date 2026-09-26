package main

import "github.com/mvrahden/go-test/internal/lint"

// Test-only exports, one block per source file.

// args.go
var (
	ExportExtractStringFlag = extractStringFlag
	ExportHasFlag           = hasFlag
	ExportKnownSubcommands  = knownSubcommands
)

// bench.go
var ExportRunBench = runBench

// cli.go
var (
	ExportParseMinFlag             = parseMinFlag
	ExportParseParallelFlag        = parseParallelFlag
	ExportParseCompileParallelFlag = parseCompileParallelFlag
	ExportParseSetupTimeoutFlag    = parseSetupTimeoutFlag
	ExportParseGlobalTimeoutFlag   = parseGlobalTimeoutFlag
	ExportResolveGlobalTimeout     = resolveGlobalTimeout
	ExportParseExecFlags           = parseExecFlags
	ExportEnsureCoverProfile       = ensureCoverProfile
)

// discover.go
type (
	ExportDiscoverOutput  = discoverOutput
	ExportDiscoverPackage = discoverPackage
	ExportDiscoverSuite   = discoverSuite
)

var (
	ExportNewDiscoverOutput  = newDiscoverOutput
	ExportBuildDiscoverSuite = buildDiscoverSuite
)

// flags.go
var (
	ExportGotestFlags    = gotestFlags
	ExportTestAllowed    = testAllowed
	ExportSpecAllowed    = specAllowed
	ExportWatchAllowed   = watchAllowed
	ExportSummaryAllowed = summaryAllowed
	ExportFuzzAllowed    = fuzzAllowed
)

// focusguard.go
var ExportDetectCIEnv = detectCIEnv

// fuzz.go
type ExportFuzzSession = fuzzSession

var (
	ExportParseForFlag      = parseForFlag
	ExportParseJobsFlag     = parseJobsFlag
	ExportPlanFuzzSession   = planFuzzSession
	ExportSelectFuzzTargets = selectFuzzTargets
)

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
var (
	ExportLintGitHubArmed = lintGitHubArmed
	ExportRunLintGitHub   = runLintGitHub
)

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
var (
	ExportParseDebounceFlag = parseDebounceFlag
	ExportIsGoFile          = isGoFile
	ExportDirsToPatterns    = dirsToPatterns
	ExportReplacePatterns   = replacePatterns
	ExportBenchDeltaLines   = benchDeltaLines
	ExportRenderWatchRun    = renderWatchRun
)
