package gotestaudit

import (
	"fmt"
	"slices"
	"strings"
)

// ClassAll names every weakening class in --fail-on.
const ClassAll = "all"

// ParseClasses reads a comma-separated list of finding classes, each once,
// in the order of Classes; "all" is every weakening.
func ParseClasses(list string) ([]string, error) {
	want := map[string]bool{}
	for _, c := range strings.Split(list, ",") {
		c = strings.TrimSpace(c)
		switch {
		case c == "":
		case c == ClassAll:
			for _, k := range Classes {
				if Weakens(k) {
					want[k] = true
				}
			}
		case slices.Contains(Classes, c):
			want[c] = true
		default:
			return nil, fmt.Errorf("unknown finding class %q; the classes are %s and %s", c, strings.Join(Classes, ", "), ClassAll)
		}
	}
	if len(want) == 0 {
		return nil, fmt.Errorf("no finding class named; the classes are %s and %s", strings.Join(Classes, ", "), ClassAll)
	}
	var out []string
	for _, k := range Classes {
		if want[k] {
			out = append(out, k)
		}
	}
	return out, nil
}

// Breaks reports whether the document holds a finding of one of classes.
func Breaks(d *Document, classes []string) bool {
	for _, c := range classes {
		if d.Summary.Classes[c] > 0 {
			return true
		}
	}
	return false
}

// ExitCode is the command's code for the document: 1 when it breaks the
// policy, else 2 when something could not be compared, else 0.
func ExitCode(d *Document, failOn []string) int {
	switch {
	case Breaks(d, failOn):
		return 1
	case len(d.Incomplete) > 0:
		return 2
	default:
		return 0
	}
}
