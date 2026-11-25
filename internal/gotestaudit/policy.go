package gotestaudit

import (
	"fmt"
	"slices"
	"strings"
)

// ClassAll names every weakening class in --fail-on, ClassProof every
// proof class. all leaves the proof classes out: they judge the runs and
// the profile a caller gives, not the change, so all keeps one meaning
// whatever evidence is given.
const (
	ClassAll   = "all"
	ClassProof = "proof"
)

// policyClasses are the classes --fail-on takes, in order.
func policyClasses() []string { return append(slices.Clone(Classes), ProofClasses...) }

// ParseClasses reads a comma-separated list of finding classes, each once,
// in the order of Classes then ProofClasses; "all" is every weakening,
// "proof" every proof class.
func ParseClasses(list string) ([]string, error) {
	known := policyClasses()
	names := strings.Join(known, ", ") + ", " + ClassAll + " and " + ClassProof
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
		case c == ClassProof:
			for _, k := range ProofClasses {
				want[k] = true
			}
		case slices.Contains(known, c):
			want[c] = true
		default:
			return nil, fmt.Errorf("unknown finding class %q; the classes are %s", c, names)
		}
	}
	if len(want) == 0 {
		return nil, fmt.Errorf("no finding class named; the classes are %s", names)
	}
	var out []string
	for _, k := range known {
		if want[k] {
			out = append(out, k)
		}
	}
	return out, nil
}

// IsProof reports whether a class judges evidence or coverage.
func IsProof(class string) bool { return slices.Contains(ProofClasses, class) }

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
