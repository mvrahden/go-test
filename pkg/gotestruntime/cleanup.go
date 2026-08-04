package gotestruntime

import (
	"flag"
	"regexp"
	"strconv"
	"strings"
)

// CountMatchingTests returns the number of executions of testNames the current
// -test.run, -test.skip and -test.count flag values will produce. Call from
// within a test function (after flag.Parse).
//
// The count feeds the fixture-teardown countdown, so its two failure modes are
// not symmetric: an undercount tears the DAG down while a fixture-bound test
// is still to run — that test then executes against released fixtures — while
// an overcount only defers teardown to process exit. Every ambiguity below
// therefore resolves high, never low.
func CountMatchingTests(testNames []string) int {
	var run, skip string
	if f := flag.Lookup("test.run"); f != nil {
		run = f.Value.String()
	}
	if f := flag.Lookup("test.skip"); f != nil {
		skip = f.Value.String()
	}
	n := countMatching(testNames, run, skip)
	// -count=n runs the whole matched set n times in one process. Each
	// execution decrements the countdown once; the fixtures stay up across
	// rounds and tear down after the last.
	if f := flag.Lookup("test.count"); f != nil {
		if c, err := strconv.Atoi(f.Value.String()); err == nil && c > 1 {
			n *= c
		}
	}
	return n
}

func countMatching(testNames []string, run, skip string) int {
	var runRe, skipRe *regexp.Regexp
	if run != "" {
		// A compile failure leaves runRe nil — no narrowing, full count.
		runRe, _ = regexp.Compile(firstPatternSegment(run))
	}
	if skip != "" && !strings.Contains(skip, "/") {
		// A skip pattern containing '/' names a subtest. The top-level test
		// still RUNS — only the subtest inside it is skipped — so excluding it
		// from the count here made the countdown hit zero one test early.
		// Ignoring such a pattern is exact, not conservative: every top-level
		// name still produces an execution.
		skipRe, _ = regexp.Compile(skip)
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

// firstPatternSegment cuts a -run pattern at the first '/' that separates test
// levels — not one inside a character class. A plain strings.Cut sheared
// `Test[a/b]` into an uncompilable half, silently disabling the filter.
// Bracket tracking mirrors go test's own splitRegexp.
func firstPatternSegment(pattern string) string {
	depth := 0
	for i, r := range pattern {
		switch r {
		case '[':
			depth++
		case ']':
			if depth > 0 {
				depth--
			}
		case '/':
			if depth == 0 {
				return pattern[:i]
			}
		}
	}
	return pattern
}
