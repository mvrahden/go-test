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
	f := testFilters{
		run:   flagValue("test.run"),
		skip:  flagValue("test.skip"),
		bench: flagValue("test.bench"),
		fuzz:  flagValue("test.fuzz"),
	}
	n := countMatching(testNames, f)
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

func flagValue(name string) string {
	if f := flag.Lookup(name); f != nil {
		return f.Value.String()
	}
	return ""
}

// testFilters are the selection flags the countdown reads.
type testFilters struct {
	run, skip, bench, fuzz string
}

func countMatching(testNames []string, f testFilters) int {
	// A compile failure leaves a pattern nil — no narrowing, full count.
	runRe := compileSegment(f.run)
	benchRe := compileSegment(f.bench)
	fuzzRe := compileSegment(f.fuzz)
	var skipRe *regexp.Regexp
	if f.skip != "" && !strings.Contains(f.skip, "/") {
		// A skip pattern containing '/' names a subtest. The top-level test
		// still RUNS — only the subtest inside it is skipped — so excluding it
		// from the count here made the countdown hit zero one test early.
		// Ignoring such a pattern is exact, not conservative: every top-level
		// name still produces an execution.
		skipRe, _ = regexp.Compile(f.skip)
	}
	count := 0
	for _, name := range testNames {
		if skipRe != nil && skipRe.MatchString(name) {
			continue
		}
		if strings.HasPrefix(name, "Benchmark") {
			// A benchmark wrapper runs only under -test.bench; bench runs pass
			// -test.run=^$, which must not count it out.
			if f.bench != "" && (benchRe == nil || benchRe.MatchString(name)) {
				count++
			}
			continue
		}
		if runRe == nil || runRe.MatchString(name) {
			count++
		}
		// Under -test.fuzz the engine calls the target once more, after the
		// seed replay -test.run may or may not have selected.
		if strings.HasPrefix(name, "Fuzz") && f.fuzz != "" && (fuzzRe == nil || fuzzRe.MatchString(name)) {
			count++
		}
	}
	if count == 0 {
		return len(testNames)
	}
	return count
}

// compileSegment compiles the top-level segment of a -test.run style pattern;
// nil for an empty or uncompilable pattern.
func compileSegment(pattern string) *regexp.Regexp {
	if pattern == "" {
		return nil
	}
	re, _ := regexp.Compile(firstPatternSegment(pattern))
	return re
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
