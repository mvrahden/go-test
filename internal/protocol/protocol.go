package protocol

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"
)

const (
	EnvSharedStateFile = "GOTEST_SHARED_STATE_FILE"
	// EnvSharedProtocolFD names the descriptor the shared fixture process
	// writes its protocol lines to; unset, they go to its stdout (Windows,
	// where exec passes no extra files).
	EnvSharedProtocolFD = "GOTEST_SHARED_PROTOCOL_FD"
	EnvUpdateSnapshots  = "GOTEST_UPDATE_SNAPSHOTS"
	EnvCI               = "GOTEST_CI"
	// EnvAgentOutput switches a text run's agent output on or off, read as
	// SwitchOn reads it; unset, a detected agent turns it on.
	EnvAgentOutput = "GOTEST_AGENT"
	// EnvLive switches a -json run's live event stream on or off, read as
	// SwitchOn reads it; unset, it is off.
	EnvLive     = "GOTEST_LIVE"
	EnvCacheDir = "GOTEST_CACHE_DIR"
	// EnvExclusiveLock at 0 lets a run's exclusive scopes overlap another
	// run's; set otherwise or unset, runs of one module take turns.
	EnvExclusiveLock = "GOTEST_EXCLUSIVE_LOCK"
	// EnvExclusiveHeld lists the lock files the run that started a process
	// holds for it, so a gotest run it starts takes them as its own.
	EnvExclusiveHeld = "GOTEST_EXCLUSIVE_HELD"
	// EnvSlowBuildThreshold is how long a build runs, as a Go duration,
	// before the CLI says it is still building.
	EnvSlowBuildThreshold = "GOTEST_SLOW_BUILD_THRESHOLD"

	// EnvFuzzEchoInput makes the codec path report every execution's decoded
	// input, not just failing ones. Set by triage/promote when re-running a
	// single corpus entry, so a no-longer-failing crasher is still readable.
	EnvFuzzEchoInput = "GOTEST_FUZZ_ECHO_INPUT"

	// EnvAssertionsFile names the file a text run's suite process appends a
	// LeafCount line to as each leaf ends: testing prints attributes only
	// under -v. The runner creates the file; the runtime takes the variable
	// out of its environment.
	EnvAssertionsFile = "GOTEST_ASSERTIONS_FILE"

	// EnvMutantOverlay names an overlay file whose replacements a run adds
	// to its harness's, and gives its suites in GOFLAGS: the mutants audit
	// --mutate compiles in.
	EnvMutantOverlay = "GOTEST_MUTANT_OVERLAY"
	// EnvSuiteCoverage names a directory where a run with -coverprofile
	// keeps each suite process's own profile and coverage data directory.
	EnvSuiteCoverage = "GOTEST_SUITE_COVERAGE"
)

// LeafCount is one line of the file EnvAssertionsFile names: what a leaf's
// attributes say, and whether it had failed when it reported them.
type LeafCount struct {
	Test       string `json:"Test"`
	Assertions int64  `json:"Assertions"`
	AssertsBy  bool   `json:"AssertsBy,omitempty"`
	Failed     bool   `json:"Failed,omitempty"`
}

// The test attributes the runtime reports on a leaf test as it ends, which go
// test -json carries as attr events: how many assertions it made, and the
// reason it declared for asserting by other means.
const (
	AttrAssertions = "gotest.assertions"
	AttrAssertsBy  = "gotest.asserts-by"
)

// ExcludedLine is the reason the runner gives a suite it skips unstarted
// because its X_ prefix, or a focus elsewhere, excludes it.
const ExcludedLine = "test suite was excluded by user"

// FuzzInputPrefix marks a line carrying the decoded input of a struct-typed
// fuzz execution. The runtime emits it on failure; triage and promote scrape it.
const FuzzInputPrefix = "ƒƒGOTEST_FUZZ_INPUT: "

const (
	SuffixFixture       = "Fixture"
	SuffixSharedFixture = "SharedFixture"
	SuffixTestSuite     = "TestSuite"
	PrefixFocused       = "F_"
	PrefixExcluded      = "X_"
	PrefixBenchmark     = "Benchmark"
	PrefixFuzz          = "Fuzz"
)

// SplitTestPath cuts a go test name into its subtest levels. The separator is a
// single slash: a run of them is left alone, so a description like
// "https:// URI" stays one level instead of becoming three. Both readers of a
// test path need this rule — the one parsing a run and the one predicting it
// from source — and they have to agree, so it lives in one place.
// A label can still hold a lone slash, so a reported name is placed by
// Placer from the stream's order; this split is its fallback for a name
// whose parent never appeared open.
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

// fuzzWrapperRe matches a generated fuzz wrapper name, Fuzz<Suite>_<Method>.
var fuzzWrapperRe = regexp.MustCompile(`^Fuzz([A-Za-z0-9_]+TestSuite)_(.+)$`)

// SplitFuzzWrapper splits a generated fuzz wrapper name into its suite type
// and method ("FuzzCartTestSuite_FuzzAdd" -> "CartTestSuite", "FuzzAdd").
func SplitFuzzWrapper(name string) (suite, method string, ok bool) {
	m := fuzzWrapperRe.FindStringSubmatch(name)
	if m == nil {
		return "", "", false
	}
	return m[1], m[2], true
}

// ParseSwitch reads an on/off variable that takes no other value: empty is
// off, a spelling of strconv.ParseBool is on or off, anything else an error
// naming the variable and the value.
func ParseSwitch(name, value string) (on bool, err error) {
	if value == "" {
		return false, nil
	}
	on, err = strconv.ParseBool(value)
	if err != nil {
		return false, fmt.Errorf("%s=%q is neither on nor off: set it to 1 or true to switch it on, to 0 or false to switch it off, or unset it", name, value)
	}
	return on, nil
}

// CheckSwitches refuses a GOTEST_CI or GOTEST_UPDATE_SNAPSHOTS value
// ParseSwitch refuses, GOTEST_CI first.
func CheckSwitches(getenv func(string) string) error {
	for _, name := range []string{EnvCI, EnvUpdateSnapshots} {
		if _, err := ParseSwitch(name, getenv(name)); err != nil {
			return err
		}
	}
	return nil
}

// SwitchOn reads the value of an on/off variable such as GOTEST_AGENT: empty
// is off, and so are the false spellings of strconv.ParseBool (0, f, F, false,
// FALSE, False). Any other value is on.
func SwitchOn(v string) bool {
	if v == "" {
		return false
	}
	on, err := strconv.ParseBool(v)
	return on || err != nil
}
