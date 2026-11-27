package gotestscenarios

import (
	"fmt"
	"slices"
	"strings"
)

// ClassUnstated names, in --fail-on, the Its no clause states.
const ClassUnstated = "unstated"

// ClassAll names, in --fail-on, every scenario status the document can
// hold: with evidence all of them, without it ambiguous and unbound.
const ClassAll = "all"

// Classes are what --fail-on can name, in the order a verdict lists them.
var Classes = []string{StatusAmbiguous, StatusFailing, StatusUnbound, StatusNotRun, StatusVacuous, ClassUnstated}

// DefaultFailOn is the policy of a document with evidence when --fail-on
// is not given.
var DefaultFailOn = []string{StatusAmbiguous, StatusFailing}

// NeedsEvidence reports a class only evidence can find.
func NeedsEvidence(class string) bool {
	return class == StatusFailing || class == StatusNotRun || class == StatusVacuous
}

// ParseClasses reads a comma-separated list of classes, each once, in the
// order of Classes. A class only evidence finds is refused without it.
func ParseClasses(list string, evidence bool) ([]string, error) {
	want := map[string]bool{}
	for _, c := range strings.Split(list, ",") {
		c = strings.TrimSpace(c)
		switch {
		case c == "":
		case c == ClassAll:
			for _, k := range Classes {
				if k != ClassUnstated && (evidence || !NeedsEvidence(k)) {
					want[k] = true
				}
			}
		case slices.Contains(Classes, c) && NeedsEvidence(c) && !evidence:
			return nil, fmt.Errorf("%s is a verdict of a run: give --input=<events file> or --verify", c)
		case slices.Contains(Classes, c):
			want[c] = true
		default:
			return nil, fmt.Errorf("unknown status %q; the statuses are %s and %s", c, strings.Join(Classes, ", "), ClassAll)
		}
	}
	if len(want) == 0 {
		return nil, fmt.Errorf("no status named; the statuses are %s and %s", strings.Join(Classes, ", "), ClassAll)
	}
	var out []string
	for _, k := range Classes {
		if want[k] {
			out = append(out, k)
		}
	}
	return out, nil
}

// RunFailed reports whether the run --verify made exited non-zero.
func (d *Document) RunFailed() bool { return d.Run != nil && d.Run.Failed }

// Breaks reports whether the document holds a scenario, or an unstated It,
// of one of classes, or a failed run of its own where failing is one.
func Breaks(d *Document, classes []string) bool {
	for _, c := range classes {
		if c == ClassUnstated && d.Summary.Unstated > 0 || d.Summary.Statuses[c] > 0 || c == StatusFailing && d.RunFailed() {
			return true
		}
	}
	return false
}

// ExitCode is the command's code: 1 when the document breaks the policy,
// else 2 when a package could not be read, else 0.
func ExitCode(d *Document, failOn []string) int {
	switch {
	case Breaks(d, failOn):
		return 1
	case len(d.Unreadable) > 0:
		return 2
	}
	return 0
}

// Verdict is the policy a document was judged by and the code it gave.
type Verdict struct {
	ExitCode int
	FailOn   []string
}
