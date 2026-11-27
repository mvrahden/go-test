package gotestrunner

import (
	"strconv"
	"strings"
)

// ShortMode reports whether the run's tests see testing.Short: -short or
// -test.short before -args, -test.short after it; the last one decides.
func ShortMode(goTestArgs []string) bool {
	short, afterArgs := false, false
	for _, w := range goTestArgs {
		if w == "-args" && !afterArgs {
			afterArgs = true
			continue
		}
		name, value, hasValue := strings.Cut(w, "=")
		if name != "-test.short" && (afterArgs || name != "-short") {
			continue
		}
		if !hasValue {
			short = true
			continue
		}
		on, err := strconv.ParseBool(value)
		short = on || err != nil
	}
	return short
}

// SelectsTests reports whether the run's flags select among each
// package's tests or can leave none to run: -run, -skip, -list, -count=0,
// and after -args -test.run, -test.skip, -test.list, -test.bench,
// -test.count=0 or a flag asking the binary for its usage.
func SelectsTests(goTestArgs []string) bool {
	return mayRunNothing(goTestArgs, ParseExecFlags(goTestArgs).RunFlags) || asksUsage(goTestArgs)
}
