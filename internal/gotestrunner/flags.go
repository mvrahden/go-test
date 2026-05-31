package gotestrunner

type ParsedFlags struct {
	BuildFlags       []string
	RunFlags         []string
	UserRunFilter    string
	UserCoverProfile string
	Verbose          bool
}

func ParseExecFlags(goTestArgs []string) ParsedFlags {
	classified := ClassifyGoTestArgs(goTestArgs)
	classified.BuildFlags = InjectChecklinkname(classified.BuildFlags)
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
	}
}
