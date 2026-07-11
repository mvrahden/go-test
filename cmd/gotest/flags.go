package main

// FlagKind indicates how a flag consumes arguments.
type FlagKind int

const (
	BoolFlag FlagKind = iota + 1
	ValueFlag
)

// gotestFlags is the central registry of all gotest-specific flags.
var gotestFlags = map[string]FlagKind{
	"--debug":              BoolFlag,
	"--ci":                 BoolFlag,
	"--spec":               BoolFlag,
	"--update-snapshots":   BoolFlag,
	"--no-color":           BoolFlag,
	"--no-cache":           BoolFlag,
	"--github":             BoolFlag,
	"--coverage":           ValueFlag,
	"--min":                ValueFlag,
	"--setup-timeout":      ValueFlag,
	"--debounce":           ValueFlag,
	"--format":             ValueFlag,
	"--output":             ValueFlag,
	"--input":              ValueFlag,
	"--parallel":           ValueFlag,
	"--compile-parallel":   ValueFlag,
	"--timeout":            ValueFlag,
}

var testAllowed = flagSet(
	"--debug", "--ci", "--spec", "--update-snapshots", "--no-cache",
	"--min", "--setup-timeout", "--timeout", "--parallel", "--compile-parallel",
)

var specAllowed = flagSet(
	"--debug", "--ci", "--update-snapshots", "--no-cache",
	"--min", "--setup-timeout", "--timeout", "--parallel", "--compile-parallel",
	"--format", "--output", "--input", "--no-color",
)

var summaryAllowed = flagSet(
	"--debug", "--ci", "--update-snapshots", "--no-cache",
	"--min", "--setup-timeout", "--timeout", "--parallel", "--compile-parallel",
	"--format", "--output", "--input", "--no-color", "--github",
	"--coverage",
)

var watchAllowed = flagSet(
	"--debug", "--ci", "--update-snapshots", "--no-cache",
	"--setup-timeout", "--timeout", "--debounce", "--parallel", "--compile-parallel",
)

func flagSet(names ...string) map[string]bool {
	s := make(map[string]bool, len(names))
	for _, n := range names {
		s[n] = true
	}
	return s
}
