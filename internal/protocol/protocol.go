package protocol

import "strings"

const (
	EnvSharedStateFile    = "GOTEST_SHARED_STATE_FILE"
	EnvTeardownBudgetFile = "GOTEST_TEARDOWN_BUDGET_FILE"
	EnvUpdateSnapshots    = "GOTEST_UPDATE_SNAPSHOTS"
	EnvCI                 = "GOTEST_CI"
	EnvCacheDir           = "GOTEST_CACHE_DIR"
)

const (
	SuffixFixture       = "Fixture"
	SuffixSharedFixture = "SharedFixture"
	SuffixTestSuite     = "TestSuite"
	PrefixFocused       = "F_"
	PrefixExcluded      = "X_"
	PrefixBenchmark     = "Benchmark"
	PrefixFuzz          = "Fuzz"
)

func BudgetFilePath(binaryPath string) string {
	return binaryPath + ".budget"
}

// SplitTestPath cuts a go test name into its subtest levels. The separator is a
// single slash: a run of them is left alone, so a description like
// "https:// URI" stays one level instead of becoming three. Both readers of a
// test path need this rule — the one parsing a run and the one predicting it
// from source — and they have to agree, so it lives in one place.
func SplitTestPath(path string) []string {
	var segments []string
	var cur strings.Builder
	for i := 0; i < len(path); i++ {
		if path[i] == '/' && (i+1 >= len(path) || path[i+1] != '/') &&
			(i == 0 || path[i-1] != '/') {
			segments = append(segments, cur.String())
			cur.Reset()
		} else {
			cur.WriteByte(path[i])
		}
	}
	if cur.Len() > 0 {
		segments = append(segments, cur.String())
	}
	return segments
}

// IsPackageSummaryLine reports whether s is a go test package-level summary
// line (e.g. "PASS", "FAIL", "ok  \tpkg\t0.01s") rather than diagnostic
// output that should be surfaced to the user.
func IsPackageSummaryLine(s string) bool {
	s = strings.TrimRight(s, "\n\r")
	return s == "PASS" || s == "FAIL" ||
		strings.HasPrefix(s, "ok  \t") ||
		strings.HasPrefix(s, "FAIL\t") ||
		strings.HasPrefix(s, "?   \t")
}
