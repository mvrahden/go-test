package main

// FlagKind indicates how a flag consumes arguments.
type FlagKind int

const (
	BoolFlag  FlagKind = iota + 1
	ValueFlag
)

// gotestFlags is the central registry of all gotest-specific flags.
var gotestFlags = map[string]FlagKind{
	"--debug":            BoolFlag,
	"--ci":               BoolFlag,
	"--spec":             BoolFlag,
	"--update-snapshots": BoolFlag,
	"--no-color":         BoolFlag,
	"--min":              ValueFlag,
	"--setup-timeout":    ValueFlag,
	"--debounce":         ValueFlag,
	"--format":           ValueFlag,
	"--output":           ValueFlag,
	"--input":            ValueFlag,
}

var testAllowed = flagSet(
	"--debug", "--ci", "--spec", "--update-snapshots",
	"--min", "--setup-timeout",
)

var specAllowed = flagSet(
	"--debug", "--ci", "--update-snapshots",
	"--min", "--setup-timeout",
	"--format", "--output", "--input", "--no-color",
)

var watchAllowed = flagSet(
	"--debug", "--ci", "--update-snapshots",
	"--setup-timeout", "--debounce",
)

func flagSet(names ...string) map[string]bool {
	s := make(map[string]bool, len(names))
	for _, n := range names {
		s[n] = true
	}
	return s
}
