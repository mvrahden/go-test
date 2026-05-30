package main

import "encoding/json"

type ExportDiscoverOutput = discoverOutput
type ExportDiscoverPackage = discoverPackage

var ExportParseMinFlag = parseMinFlag
var ExportBuildDiscoverSuite = buildDiscoverSuite
var ExportExtractStringFlag = extractStringFlag
var ExportHasFlag = hasFlag
var ExportIsGoFile = isGoFile
var ExportDirsToPatterns = dirsToPatterns
var ExportReplacePatterns = replacePatterns
var ExportTestAllowed = testAllowed
var ExportSpecAllowed = specAllowed
var ExportWatchAllowed = watchAllowed

var ExportAssignCoverProfiles = assignCoverProfiles
var ExportBuildExtraEnv = buildExtraEnv

type ExportExecConfig = ExecConfig
type ExportFixtureStateEntry = fixtureStateEntry
type ExportSharedFixtureProcess = SharedFixtureProcess

// ExportParsedFlags is an exported view of parsedFlags for test access.
type ExportParsedFlags struct {
	BuildFlags       []string
	RunFlags         []string
	UserRunFilter    string
	UserCoverProfile string
	Verbose          bool
}

// ExportParseExecFlags wraps parseExecFlags and converts to an exported struct.
func ExportParseExecFlags(goTestArgs []string) ExportParsedFlags {
	pf := parseExecFlags(goTestArgs)
	return ExportParsedFlags{
		BuildFlags:       pf.buildFlags,
		RunFlags:         pf.runFlags,
		UserRunFilter:    pf.userRunFilter,
		UserCoverProfile: pf.userCoverProfile,
		Verbose:          pf.verbose,
	}
}

// ExportNewSharedFixtureProcess constructs a SharedFixtureProcess for testing.
func ExportNewSharedFixtureProcess(sharedDir string, state map[string]json.RawMessage) *SharedFixtureProcess {
	return &SharedFixtureProcess{
		sharedDir: sharedDir,
		state:     state,
	}
}
