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
