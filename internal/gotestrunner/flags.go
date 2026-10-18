package gotestrunner

type ParsedFlags struct {
	BuildFlags       []string
	RunFlags         []string
	UserRunFilter    string
	UserCoverProfile string
	Verbose          bool
	// Exec is go test's -exec: the command every suite binary runs under.
	Exec []string
	// ProcessCap is go test's -p: at most this many suite processes at once.
	ProcessCap int
}

func ParseExecFlags(goTestArgs []string) ParsedFlags {
	classified := ClassifyGoTestArgs(goTestArgs)
	execPrefix, buildFlags := ExtractExecPrefix(classified.BuildFlags)
	classified.BuildFlags = buildFlags
	verbose := HasVerboseFlag(classified.RunFlags)
	userRunFilter := ExtractRunFilter(classified.RunFlags)
	runFlags := StripRunFilter(classified.RunFlags)
	userCoverProfile := ExtractCoverProfile(runFlags)
	runFlags = StripCoverProfile(runFlags)
	runFlags = InjectDefaultTimeout(runFlags)
	return ParsedFlags{
		BuildFlags:       classified.BuildFlags,
		RunFlags:         runFlags,
		UserRunFilter:    userRunFilter,
		UserCoverProfile: userCoverProfile,
		Verbose:          verbose,
		Exec:             execPrefix,
		ProcessCap:       ExtractProcessCap(classified.BuildFlags),
	}
}
