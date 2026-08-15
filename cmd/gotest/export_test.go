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
var ExportFuzzAllowed = fuzzAllowed
var ExportParseExecFlags = parseExecFlags
