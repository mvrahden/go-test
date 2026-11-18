package gotestrunner

import "strings"

// profileFlags each name one file every suite process would write; unlike
// -coverprofile, nothing merges them.
var profileFlags = []string{"-cpuprofile", "-memprofile", "-blockprofile", "-mutexprofile", "-trace"}

// sharedProfileFlag returns the first profile flag in runFlags before -args,
// spelled with its single dash, or "".
func sharedProfileFlag(runFlags []string) string {
	for _, f := range runFlags {
		if f == "-args" {
			return ""
		}
		name, _, _ := strings.Cut(f, "=")
		name = strings.Replace(name, "-test.", "-", 1)
		for _, p := range profileFlags {
			if name == p {
				return p
			}
		}
	}
	return ""
}

// plannedSuiteCount is the number of suite processes the scopes dispatch.
func plannedSuiteCount(scopes []suiteScope) int {
	n := 0
	for _, sc := range scopes {
		for _, suites := range sc.Bulk {
			n += len(suites)
		}
		for _, suites := range sc.Tail {
			n += len(suites)
		}
	}
	return n
}
