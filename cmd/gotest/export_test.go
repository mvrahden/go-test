package main

import "github.com/mvrahden/go-test/internal/lint"

// ExportResetLintSkipFlag restores an analyzer skip flag after a test has
// set it through the GitHub lint mode; the flag set is process-global.
func ExportResetLintSkipFlag(name string) error {
	return lint.Analyzer.Flags.Set(name, "false")
}

type ExportDiscoverOutput = discoverOutput
type ExportDiscoverPackage = discoverPackage
type ExportDiscoverSuite = discoverSuite

var ExportParseMinFlag = parseMinFlag
var ExportRunSpecFromInput = runSpecFromInput
var ExportRunSummaryFromInput = runSummaryFromInput
var ExportRunSpec = runSpec
var ExportRunStaticSpec = runStaticSpec
var ExportRunSummary = runSummary
var ExportParseParallelFlag = parseParallelFlag
var ExportParseCompileParallelFlag = parseCompileParallelFlag
var ExportParseSetupTimeoutFlag = parseSetupTimeoutFlag
var ExportParseGlobalTimeoutFlag = parseGlobalTimeoutFlag
var ExportResolveGlobalTimeout = resolveGlobalTimeout
var ExportParseDebounceFlag = parseDebounceFlag
var ExportBuildDiscoverSuite = buildDiscoverSuite
var ExportExtractStringFlag = extractStringFlag
var ExportHasFlag = hasFlag
var ExportIsGoFile = isGoFile
var ExportDirsToPatterns = dirsToPatterns
var ExportReplacePatterns = replacePatterns
var ExportRunScaffold = runScaffold
var ExportDetectCIEnv = detectCIEnv
var ExportKnownSubcommands = knownSubcommands
var ExportLintGitHubArmed = lintGitHubArmed
var ExportRunLintGitHub = runLintGitHub
var ExportGotestFlags = gotestFlags
var ExportTestAllowed = testAllowed
var ExportSpecAllowed = specAllowed
var ExportWatchAllowed = watchAllowed
var ExportSummaryAllowed = summaryAllowed
var ExportEnsureCoverProfile = ensureCoverProfile
var ExportBenchDeltaLines = benchDeltaLines

var ExportFuzzAllowed = fuzzAllowed
var ExportParseExecFlags = parseExecFlags
var ExportParseForFlag = parseForFlag
var ExportPlanFuzzSession = planFuzzSession

type ExportFuzzSession = fuzzSession

var ExportParseJobsFlag = parseJobsFlag

type ExportCensusCase = censusCase

var ExportExecutedCases = executedCases
var ExportCensusMissing = censusMissing
var ExportEnforceCensus = enforceCensus
var ExportExecutedBenchCases = executedBenchCases
var ExportEnforceBenchCensus = enforceBenchCensus
var ExportRunBench = runBench
