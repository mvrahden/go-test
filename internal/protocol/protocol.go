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
)

func BudgetFilePath(binaryPath string) string {
	return binaryPath + ".budget"
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
