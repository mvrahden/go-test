package gotestruntime

import (
	"flag"
	"regexp"
	"strings"
)

// CountMatchingTests returns the number of testNames that match the current
// -test.run and -test.skip flag values. Call from within a test function
// (after flag.Parse). Returns len(testNames) when no flags narrow the set
// or when no names match (safety fallback).
func CountMatchingTests(testNames []string) int {
	var run, skip string
	if f := flag.Lookup("test.run"); f != nil {
		run = f.Value.String()
	}
	if f := flag.Lookup("test.skip"); f != nil {
		skip = f.Value.String()
	}
	return countMatching(testNames, run, skip)
}

func countMatching(testNames []string, run, skip string) int {
	var runRe, skipRe *regexp.Regexp
	if run != "" {
		seg, _, _ := strings.Cut(run, "/")
		runRe, _ = regexp.Compile(seg)
	}
	if skip != "" {
		seg, _, _ := strings.Cut(skip, "/")
		skipRe, _ = regexp.Compile(seg)
	}
	if runRe == nil && skipRe == nil {
		return len(testNames)
	}
	count := 0
	for _, name := range testNames {
		if runRe != nil && !runRe.MatchString(name) {
			continue
		}
		if skipRe != nil && skipRe.MatchString(name) {
			continue
		}
		count++
	}
	if count == 0 {
		return len(testNames)
	}
	return count
}
