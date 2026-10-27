package main

import (
	"fmt"
	"maps"
	"os"
)

// FlagKind indicates how a flag consumes arguments.
type FlagKind int

const (
	BoolFlag FlagKind = iota + 1
	ValueFlag
	// ListFlag is a value flag that may repeat: every occurrence counts.
	ListFlag
)

// gotestFlags is the central registry of all gotest-specific flags.
var gotestFlags = map[string]FlagKind{
	"--debug":            BoolFlag,
	"--ci":               BoolFlag,
	"--spec":             BoolFlag,
	"--update-snapshots": BoolFlag,
	"--no-color":         BoolFlag,
	"--no-cache":         BoolFlag,
	"--github":           BoolFlag,
	"--render-only":      BoolFlag,
	"--static":           BoolFlag,
	"--coverage":         ValueFlag,
	"--badge":            ValueFlag,
	"--min":              ValueFlag,
	"--setup-timeout":    ValueFlag,
	"--debounce":         ValueFlag,
	"--format":           ValueFlag,
	"--output":           ValueFlag,
	"--input":            ValueFlag,
	"--events-file":      ValueFlag,
	"--parallel":         ValueFlag,
	"--compile-parallel": ValueFlag,
	"--timeout":          ValueFlag,
	"--save":             ValueFlag,
	"--against":          ValueFlag,
	"--gate":             ValueFlag,
	"--bench":            BoolFlag,
	"--json":             BoolFlag,

	"--for":        ValueFlag,
	"--jobs":       ValueFlag,
	"--target":     ValueFlag,
	"--no-harvest": BoolFlag,
	"--fuzz":       BoolFlag,
	"--dry-run":    BoolFlag,
	"--dir":        ValueFlag,

	"--group":      ListFlag,
	"--skip-group": ListFlag,
}

var testAllowed = flagSet(
	"--debug", "--ci", "--spec", "--update-snapshots", "--no-cache",
	"--min", "--setup-timeout", "--timeout", "--parallel", "--compile-parallel",
	"--no-harvest", "--events-file", "--group", "--skip-group",
)

var specAllowed = flagSet(
	"--debug", "--ci", "--update-snapshots", "--no-cache",
	"--min", "--setup-timeout", "--timeout", "--parallel", "--compile-parallel",
	"--format", "--output", "--input", "--no-color", "--render-only",
	"--static", "--group", "--skip-group",
)

var summaryAllowed = flagSet(
	"--debug", "--ci", "--update-snapshots", "--no-cache",
	"--min", "--setup-timeout", "--timeout", "--parallel", "--compile-parallel",
	"--format", "--output", "--input", "--no-color", "--github",
	"--coverage", "--render-only", "--badge", "--events-file",
	"--group", "--skip-group",
)

var watchAllowed = flagSet(
	"--debug", "--ci", "--update-snapshots", "--no-cache", "--spec",
	"--setup-timeout", "--timeout", "--debounce", "--parallel", "--compile-parallel",
	"--bench",
)

var benchAllowed = flagSet(
	"--timeout", "--setup-timeout", "--no-cache", "--debug", "--spec", "--no-color",
	"--save", "--against", "--gate", "--json",
)

var fuzzAllowed = flagSet("--for", "--jobs", "--no-cache", "--debug", "--no-harvest", "--target")

var scaffoldAllowed = flagSet("--fuzz")

// noOwnFlags is the allowed set of the commands that take only packages and
// go test flags: any gotest flag is refused, an unknown one is still unknown.
var noOwnFlags = flagSet()

var migrateAllowed = flagSet("--dry-run")

var skillAllowed = flagSet("--dir")

func flagSet(names ...string) map[string]bool {
	s := make(map[string]bool, len(names))
	for _, n := range names {
		s[n] = true
	}
	return s
}

// OwnFlags is one invocation's gotest flags after parsing: each flag's final
// value, the last occurrence winning, as the go command reads its own flags.
// A boolean holds "true" or "false"; a value flag holds its text, which may
// be empty when it was given as --name=.
type OwnFlags struct {
	values map[string]string
	lists  map[string][]string
}

func (f *OwnFlags) set(name, value string) {
	if f.values == nil {
		f.values = map[string]string{}
	}
	f.values[name] = value
}

func (f *OwnFlags) add(name, value string) {
	f.set(name, value)
	if f.lists == nil {
		f.lists = map[string][]string{}
	}
	f.lists[name] = append(f.lists[name], value)
}

// refuseGroups reports, on stderr, --group or --skip-group given where mode
// runs no suite.
func refuseGroups(own OwnFlags, mode string) bool {
	if !own.Given("--group") && !own.Given("--skip-group") {
		return false
	}
	fmt.Fprintf(os.Stderr, "FAIL: --group and --skip-group select the suites a run runs, and %s runs nothing\n", mode)
	return true
}

// Values is every value a repeatable flag was given, in order.
func (f OwnFlags) Values(name string) []string { return f.lists[name] }

// Given reports whether the flag appeared at all.
func (f OwnFlags) Given(name string) bool {
	_, ok := f.values[name]
	return ok
}

// Bool is a boolean flag's final value; an absent flag is false.
func (f OwnFlags) Bool(name string) bool { return f.values[name] == "true" }

// Value is a value flag's final text; an absent flag is "".
func (f OwnFlags) Value(name string) string { return f.values[name] }

// ValueOr is Value with a default for an absent or empty flag.
func (f OwnFlags) ValueOr(name, def string) string {
	if v := f.values[name]; v != "" {
		return v
	}
	return def
}

// Snapshot is a copy of every parsed flag, nil when none was given.
func (f OwnFlags) Snapshot() map[string]string {
	if len(f.values) == 0 {
		return nil
	}
	return maps.Clone(f.values)
}
