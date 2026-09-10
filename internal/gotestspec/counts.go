package gotestspec

import "fmt"

// countNoun spells a count with its noun, singular for exactly one.
func countNoun(n int, singular, plural string) string {
	if n == 1 {
		return fmt.Sprintf("1 %s", singular)
	}
	return fmt.Sprintf("%d %s", n, plural)
}

// countParts lists the populated kinds of a run in trailer order, or "0 suites"
// when nothing was found at all.
func countParts(stats Stats) []string {
	var counts []string
	if stats.Suites > 0 {
		counts = append(counts, countNoun(stats.Suites, "suite", "suites"))
	}
	if stats.Behaviors > 0 {
		counts = append(counts, countNoun(stats.Behaviors, "behavior", "behaviors"))
	}
	if stats.Tests > 0 {
		counts = append(counts, countNoun(stats.Tests, "stdlib test", "stdlib tests"))
	}
	if stats.Benchmarks > 0 {
		counts = append(counts, countNoun(stats.Benchmarks, "benchmark", "benchmarks"))
	}
	if stats.Fuzzers > 0 {
		counts = append(counts, countNoun(stats.Fuzzers, "fuzz target", "fuzz targets"))
	}
	if len(counts) == 0 {
		counts = append(counts, "0 suites")
	}
	return counts
}
