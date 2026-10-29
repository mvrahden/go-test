// Ring 0: raw checks only, so a broken assertion engine cannot pass its own
// tests. No gotest.* assertion may appear in this file. The canary anchors
// verdicts outside gotest's reporting stack: it runs the built CLI over
// fixture packages and checks exit codes, the raw go test -json stream and
// marker files against a checked-in golden list, with plain Go only.
package canary_test //nolint:fail-guard

import (
	"bufio"
	"bytes"
	"encoding/json"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"slices"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/mvrahden/go-test/internal/protocol"
	"github.com/mvrahden/go-test/internal/testkit"
	"github.com/mvrahden/go-test/pkg/gotest"
)

// canary is what every canary suite holds: the CLI it built and the golden
// list. The canary is split into suites only so that the runner runs them
// side by side, each in its own process; none runs its methods in parallel,
// so the canary does not rest on the parallel machinery it guards.
type canary struct {
	binary   string
	expected map[string]expectation
}

type expectation struct {
	exit int
	rows []string
}

func fatalf(t *gotest.T, format string, args ...any) {
	t.Errorf(format, args...)
	t.FailNow()
}

// open builds the CLI and reads the golden list; each suite's BeforeAll.
func (s *canary) open(t *gotest.T) {
	root, err := filepath.Abs("../..")
	if err != nil {
		fatalf(t, "abs: %v", err)
	}
	// The plain build, never the shared fixture: the canary guards that machinery.
	s.binary, err = testkit.BuildCLI(t.Context(), root, t.TempDir())
	if err != nil {
		fatalf(t, "%v", err)
	}
	testkit.ScrubActionsEnv()
	s.expected = readExpected(t, filepath.Join("testdata", "expected.txt"))
}

// CanaryTestSuite runs one fixture package per method: verdicts, assertions,
// fixtures, goroutines, FailFast and benchmarks.
// Sequential: it builds the CLI once and each method spawns a full pipeline run.
type CanaryTestSuite struct{ canary }

func (s *CanaryTestSuite) SuiteConfig() gotest.SuiteConfig {
	// Slowest method ~8s locally, setup ~13s on Windows CI; budgets are twice the Windows estimate, rounded up.
	return gotest.SuiteConfig{Timeout: time.Minute, SetupTimeout: 30 * time.Second}
}

func (s *CanaryTestSuite) BeforeAll(t *gotest.T) { s.open(t) }

func (s *CanaryTestSuite) AfterAll(t *gotest.T) {}

// readExpected parses the golden list: a "<pkg> exit <code>" line opens a
// package, and "<pkg> <action> <test>" lines follow it.
func readExpected(t *gotest.T, path string) map[string]expectation {
	f, err := os.Open(path)
	if err != nil {
		fatalf(t, "open golden: %v", err)
	}
	defer f.Close()
	out := map[string]expectation{}
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		fields := strings.Fields(sc.Text())
		if len(fields) != 3 {
			continue
		}
		pkg := fields[0]
		e := out[pkg]
		if fields[1] == "exit" {
			code, err := strconv.Atoi(fields[2])
			if err != nil {
				fatalf(t, "golden exit code for %s: %v", pkg, err)
			}
			e.exit = code
		} else {
			e.rows = append(e.rows, strings.Join(fields, " "))
		}
		out[pkg] = e
	}
	if err := sc.Err(); err != nil {
		fatalf(t, "read golden: %v", err)
	}
	return out
}

// result is what one CLI run over a fixture left.
type result struct {
	code   int
	rows   []string // the verdicts, in golden form
	stream string   // the -json stdout, for checks on what it carried
	stderr string
	dir    string // the canary dir
}

type event struct {
	Action  string `json:"Action"`
	Package string `json:"Package"`
	Test    string `json:"Test"`
}

// run executes the CLI in -json mode over one fixture package.
func (s *canary) run(t *gotest.T, pkg, canaryDir string) result {
	return s.runWith(t, pkg, canaryDir, nil)
}

// runWith is run with env on top of the child's environment and flags
// before the package.
func (s *canary) runWith(t *gotest.T, pkg, canaryDir string, env []string, flags ...string) result {
	return s.runIn(t, "", "./testdata/"+pkg+"/", pkg, canaryDir, env, flags...)
}

// runIn is runWith over the package at path, run from dir ("" for this
// one); its rows are named pkg.
func (s *canary) runIn(t *gotest.T, dir, path, pkg, canaryDir string, env []string, flags ...string) result {
	args := append(append([]string{"-json"}, flags...), path)
	cmd := exec.Command(s.binary, args...) //nolint:gosec // G204: controlled binary with fixed args
	cmd.Dir = dir
	cmd.Env = childEnv(canaryDir, env...)
	var stdout, stderr bytes.Buffer
	cmd.Stdout, cmd.Stderr = &stdout, &stderr
	code := 0
	if err := cmd.Run(); err != nil {
		var exitErr *exec.ExitError
		if !errors.As(err, &exitErr) {
			fatalf(t, "run %s: %v", pkg, err)
		}
		code = exitErr.ExitCode()
	}
	seen := map[string]bool{}
	for _, line := range bytes.Split(stdout.Bytes(), []byte("\n")) {
		if !bytes.HasPrefix(line, []byte("{")) {
			continue
		}
		var ev event
		if err := json.Unmarshal(line, &ev); err != nil {
			continue
		}
		if ev.Action != "pass" && ev.Action != "fail" && ev.Action != "skip" {
			continue
		}
		test := ev.Test
		if test == "" {
			test = "-"
		}
		seen[pkg+" "+ev.Action+" "+test] = true
	}
	rows := make([]string, 0, len(seen))
	for r := range seen {
		rows = append(rows, r)
	}
	sort.Strings(rows)
	return result{code: code, rows: rows, stream: stdout.String(), stderr: stderr.String(), dir: canaryDir}
}

// childEnv is the fixture process's environment: the canary dir, its own
// gotest cache, and none of the host's CI or GOTEST_* settings, so a run
// checks the same on a CI runner as on a laptop. Under CI snapshots are
// read-only, and a refresh of the repository's own snapshots must not
// rewrite the snapshotting fixture's deliberately stale baseline. Its temp
// dir is inside the canary dir, so its sweep never reaches the real one's
// dirs. extra pairs are what a case sets on purpose; they go on top. The
// slow-build threshold passes: it describes the machine, not the run.
func childEnv(canaryDir string, extra ...string) []string {
	env := make([]string, 0, len(os.Environ())+5+len(extra))
	for _, kv := range os.Environ() {
		name, _, _ := strings.Cut(kv, "=")
		gotestVar := strings.HasPrefix(name, "GOTEST_") && name != protocol.EnvSlowBuildThreshold
		if gotestVar || slices.Contains(hostOnlyEnv, name) || slices.ContainsFunc(tempEnv, func(v string) bool { return strings.EqualFold(v, name) }) {
			continue
		}
		env = append(env, kv)
	}
	tmp := filepath.Join(canaryDir, "tmp")
	// A failure here fails the run, which cannot make its work dir.
	_ = os.MkdirAll(tmp, 0o700)
	env = append(env, "GOTEST_CANARY_DIR="+canaryDir, "GOTEST_CACHE_DIR="+testkit.ChildCacheDir(canaryDir),
		"TMPDIR="+tmp, "TMP="+tmp, "TEMP="+tmp)
	return append(env, extra...)
}

func (s *CanaryTestSuite) TestTheChildEnvKeepsOnlyTheSlowBuildThreshold(t *gotest.T) {
	t.Setenv(protocol.EnvSlowBuildThreshold, "1ns")
	t.Setenv(protocol.EnvCI, "1")
	env := childEnv(t.TempDir())
	if !slices.Contains(env, protocol.EnvSlowBuildThreshold+"=1ns") {
		t.Errorf("the slow-build threshold does not reach the fixture process")
	}
	if slices.Contains(env, protocol.EnvCI+"=1") {
		t.Errorf("the host's %s reaches the fixture process", protocol.EnvCI)
	}
}

// tempEnv name the temp dir; Windows spells them in any case.
var tempEnv = []string{"TMPDIR", "TMP", "TEMP"}

// hostOnlyEnv are the variables that put the CLI in CI mode or point it at
// the job's step summary.
var hostOnlyEnv = []string{"CI", "GITHUB_ACTIONS", "GITHUB_STEP_SUMMARY"}

// check runs a fixture, with flags before the package, and compares exit
// code and rows with the golden.
func (s *canary) check(t *gotest.T, pkg string, flags ...string) result {
	return s.checkIn(t, pkg, t.TempDir(), flags...)
}

// checkIn is check with dir as the canary dir.
func (s *canary) checkIn(t *gotest.T, pkg, dir string, flags ...string) result {
	want, ok := s.expected[pkg]
	if !ok {
		fatalf(t, "no golden entry for %s", pkg)
	}
	r := s.runWith(t, pkg, dir, nil, flags...)
	if r.code != want.exit {
		t.Errorf("%s: exit code %d, want %d", pkg, r.code, want.exit)
	}
	if strings.Join(r.rows, "\n") != strings.Join(want.rows, "\n") {
		t.Errorf("%s: verdicts differ from the golden list\n got:\n  %s\n want:\n  %s",
			pkg, strings.Join(r.rows, "\n  "), strings.Join(want.rows, "\n  "))
	}
	return r
}

func (s *CanaryTestSuite) TestPassingSuiteIsGreen(t *gotest.T) {
	s.check(t, "passing")
}

// A fixture run leaves the host's gotest cache and CI settings alone: its
// entries land in its own cache, and only what a case sets reaches it.
func (s *CanaryTestSuite) TestAFixtureRunKeepsToItsOwnCacheAndEnv(t *gotest.T) {
	host, hostTmp := t.TempDir(), t.TempDir()
	t.Setenv("GOTEST_CACHE_DIR", host)
	t.Setenv("CI", "true")
	t.Setenv("GOTEST_CI", "1")
	dir := s.check(t, "passing").dir
	t.Setenv("TMPDIR", hostTmp)
	if entries, err := os.ReadDir(host); err != nil || len(entries) != 0 {
		t.Errorf("host gotest cache: %d entries (err %v), want none", len(entries), err)
	}
	if entries, err := os.ReadDir(testkit.ChildCacheDir(dir)); err != nil || len(entries) == 0 {
		t.Errorf("child gotest cache: %d entries (err %v), want some", len(entries), err)
	}
	env := childEnv(dir, "GOTEST_CI=0")
	for _, kv := range env {
		if kv == "CI=true" || kv == "GOTEST_CI=1" || kv == "GOTEST_CACHE_DIR="+host {
			t.Errorf("child env inherits the host's %s", kv)
		}
	}
	for _, name := range tempEnv {
		if !slices.Contains(env, name+"="+filepath.Join(dir, "tmp")) {
			t.Errorf("child env names no temp dir of its own as %s, so its sweep reaches other runs' dirs", name)
		}
	}
	if slices.Contains(env, "TMPDIR="+hostTmp) {
		t.Errorf("child env keeps the host's temp dir")
	}
	if env[len(env)-1] != "GOTEST_CI=0" {
		t.Errorf("child env drops what the case set: ends with %q", env[len(env)-1])
	}
}

// The same fixture checks verdicts and diagnostic content in one run.
func (s *CanaryTestSuite) TestEveryAssertionFamilyFails(t *gotest.T) {
	r := s.check(t, "failing")
	for _, want := range []string{"is of type *fs.PathError", "[]int cannot hold a value of type string", "42 retries left", "delta is NaN", "diff (-expected +actual):", `    -c\n`, `    +C\n`, "poll still running past its deadline, abandoned 100ms after it", "must be positive", "must be one line"} {
		if !strings.Contains(r.stream, want) {
			t.Errorf("failing: no failure message says %q", want)
		}
	}
}

// A failure inside a helper names the helper's line and the caller's, also
// under -trimpath in a module whose path has no dot, which the standard
// library's import paths resemble.
func (s *CanaryTestSuite) TestAHelperFailureNamesBothLinesUnderTrimpath(t *gotest.T) {
	root, err := filepath.Abs("../..")
	if err != nil {
		fatalf(t, "abs: %v", err)
	}
	dir := t.TempDir()
	if _, err := testkit.StagePackage(root, filepath.Join("testdata", "helpers"), dir, "plainmod"); err != nil {
		fatalf(t, "stage helpers: %v", err)
	}
	run := exec.Command(s.binary, "-trimpath", "-v", "./helpers/") //nolint:gosec // G204: controlled binary with fixed args
	run.Dir = dir
	run.Env = childEnv(t.TempDir())
	out, _ := run.CombinedOutput()
	if !strings.Contains(string(out), "helpers_suite_test.go:14: helpers_suite_test.go:10: Equal failed") {
		t.Errorf("helpers under -trimpath: the failure does not name both lines:\n%s", out)
	}
}

// A baseline the value no longer matches must fail; MatchSnapshot comparing
// nothing would leave every golden check in the repository silent.
func (s *CanaryTestSuite) TestASnapshotMismatchFails(t *gotest.T) {
	s.check(t, "snapshotting")
}

// stage copies fixture pkg into a module of its own under a temp dir and
// returns that dir, where a run that writes baselines leaves the source
// tree alone.
func stage(t *gotest.T, pkg string) string {
	root, err := filepath.Abs("../..")
	if err != nil {
		fatalf(t, "abs: %v", err)
	}
	dir := t.TempDir()
	if _, err := testkit.StagePackage(root, filepath.Join("testdata", pkg), dir, "canary"+pkg); err != nil {
		fatalf(t, "stage %s: %v", pkg, err)
	}
	return dir
}

// A snapshot section is the exact name testing gave the subtest, found
// beside the test under -trimpath too; an update switch spelled off is off.
// GOTEST_CI=1 keeps a baseline that is not found from being written.
func (s *CanaryTestSuite) TestSnapshotsKeyOnTheExactName(t *gotest.T) {
	want := s.expected["snapshotting"]
	for _, c := range []struct {
		how   string
		env   []string
		flags []string
	}{
		{"built with -trimpath", nil, []string{"-trimpath"}},
		{"with GOTEST_UPDATE_SNAPSHOTS=0", []string{"GOTEST_UPDATE_SNAPSHOTS=0"}, nil},
		{"with GOTEST_UPDATE_SNAPSHOTS=false", []string{"GOTEST_UPDATE_SNAPSHOTS=false"}, nil},
		{"with GOTEST_UPDATE_SNAPSHOTS=FALSE", []string{"GOTEST_UPDATE_SNAPSHOTS=FALSE"}, nil},
		{"with GOTEST_UPDATE_SNAPSHOTS=f", []string{"GOTEST_UPDATE_SNAPSHOTS=f"}, nil},
	} {
		dir := stage(t, "snapshotting")
		baseline := filepath.Join(dir, "snapshotting", "testdata", "__snapshots__", "TestSnapshottingTestSuite_ext.snap")
		before, err := os.ReadFile(baseline) //nolint:gosec // G304: the staged copy of the canary's own fixture
		if err != nil {
			fatalf(t, "baseline: %v", err)
		}
		r := s.runIn(t, dir, "./snapshotting/", "snapshotting", t.TempDir(), append([]string{"GOTEST_CI=1"}, c.env...), c.flags...)
		if r.code != want.exit || strings.Join(r.rows, "\n") != strings.Join(want.rows, "\n") {
			t.Errorf("snapshotting %s: verdicts differ (exit %d)\n got:\n  %s", c.how, r.code, strings.Join(r.rows, "\n  "))
		}
		if after, _ := os.ReadFile(baseline); !bytes.Equal(after, before) { //nolint:gosec // G304: as above
			t.Errorf("snapshotting %s: the baseline was rewritten", c.how)
		}
	}
}

// legacyRows are the verdicts of the snapshotlegacy fixture's tests, each
// keyed by its path below the suite.
func legacyRows(rows []string) map[string]string {
	out := map[string]string{}
	for _, r := range rows {
		f := strings.Fields(r)
		if len(f) == 3 {
			out[strings.TrimPrefix(f[2], "TestSnapLegacyTestSuite/")] = f[1]
		}
	}
	return out
}

// A baseline keyed the way the release before exact names keyed it is
// compared under the new key: read-only under GOTEST_CI, renamed to the new
// key otherwise. A section the exact key of another test holds serves only
// that test, and GOTEST_CI spelled FALSE is off.
func (s *CanaryTestSuite) TestOldSnapshotKeysAreMigrated(t *gotest.T) {
	dir := stage(t, "snapshotlegacy")
	baseline := filepath.Join(dir, "snapshotlegacy", "testdata", "__snapshots__", "TestSnapLegacyTestSuite_ext.snap")
	read := func() string {
		data, err := os.ReadFile(baseline) //nolint:gosec // G304: the staged copy of the canary's own fixture
		if err != nil {
			fatalf(t, "baseline: %v", err)
		}
		return string(data)
	}
	before := read()
	for _, c := range []struct {
		how  string
		env  string
		want map[string]string
		file string
	}{
		{"under GOTEST_CI=1", "GOTEST_CI=1",
			map[string]string{"TestIssue/issue_#42": "pass", "TestRows/#0": "pass", "TestTwins/x": "pass", "TestTwins/x#01": "fail", "TestStale/issue_#7": "fail"},
			before},
		{"under GOTEST_CI=FALSE", "GOTEST_CI=FALSE",
			map[string]string{"TestIssue/issue_#42": "pass", "TestRows/#0": "pass", "TestTwins/x": "pass", "TestTwins/x#01": "pass", "TestStale/issue_#7": "fail"},
			"=== SNAP TestIssue/issue_#42 ===\nforty-two\n=== SNAP TestRows/#0 n ===\nrow zero\n=== SNAP TestStale/issue_ ===\nold\n=== SNAP TestTwins/x ===\nfirst twin\n=== SNAP TestTwins/x#01 ===\nsecond twin\n"},
		{"under GOTEST_CI=1 once renamed", "GOTEST_CI=1",
			map[string]string{"TestIssue/issue_#42": "pass", "TestRows/#0": "pass", "TestTwins/x": "pass", "TestTwins/x#01": "pass", "TestStale/issue_#7": "fail"},
			""},
	} {
		r := s.runIn(t, dir, "./snapshotlegacy/", "snapshotlegacy", t.TempDir(), []string{c.env})
		got := legacyRows(r.rows)
		for name, verdict := range c.want {
			if got[name] != verdict {
				t.Errorf("snapshotlegacy %s: %s got %q, want %q", c.how, name, got[name], verdict)
			}
		}
		if after := read(); c.file != "" && after != c.file {
			t.Errorf("snapshotlegacy %s: baseline file\n got:\n%s\n want:\n%s", c.how, after, c.file)
		}
	}
}

// An Exclusive suite is dispatched strictly alone, after every non-exclusive
// suite has drained (AGENTS.md). The fixture's three suites stamp their
// windows; the exclusive one must open after both bulk windows closed.
func (s *CanaryTestSuite) TestAnExclusiveSuiteRunsAfterTheBulk(t *gotest.T) {
	dir := s.check(t, "exclusive").dir
	windows := readWindows(t, filepath.Join(dir, "windows"))
	exclusiveStart := windows["start ExclusiveTestSuite"]
	for _, bulk := range []string{"BulkOneTestSuite", "BulkTwoTestSuite"} {
		if end := windows["end "+bulk]; end == 0 || exclusiveStart <= end {
			t.Errorf("exclusive: ExclusiveTestSuite started before the bulk drained (%s ended at %d, exclusive started at %d)", bulk, end, exclusiveStart)
		}
	}
}

// readWindows parses "<event> <suite> <unixnano>" lines into "<event> <suite>" keys.
func readWindows(t *gotest.T, path string) map[string]int64 {
	data, err := os.ReadFile(path)
	if err != nil {
		fatalf(t, "read windows: %v", err)
	}
	windows := map[string]int64{}
	for _, line := range strings.Split(strings.TrimSpace(string(data)), "\n") {
		fields := strings.Fields(line)
		if len(fields) != 3 {
			fatalf(t, "malformed window line %q", line)
		}
		ns, err := strconv.ParseInt(fields[2], 10, 64)
		if err != nil {
			fatalf(t, "malformed window stamp %q: %v", line, err)
		}
		windows[fields[0]+" "+fields[1]] = ns
	}
	return windows
}

func (s *CanaryTestSuite) TestAnAssertionHaltsTheMethod(t *gotest.T) {
	dir := s.check(t, "failnow").dir
	if _, err := os.Stat(filepath.Join(dir, "after-failnow")); err == nil {
		t.Errorf("the statement after a failed assertion ran: the assertion did not halt")
	}
}

func (s *CanaryTestSuite) TestAfterEachRunsWhenTheTestFailed(t *gotest.T) {
	dir := s.check(t, "aftereach").dir
	if _, err := os.Stat(filepath.Join(dir, "aftereach-ran")); err != nil {
		t.Errorf("AfterEach did not run after a failing test: %v", err)
	}
}

func (s *CanaryTestSuite) TestLifecycleRunsInOrder(t *gotest.T) {
	dir := s.check(t, "lifecycle").dir
	got, err := os.ReadFile(filepath.Join(dir, "lifecycle"))
	if err != nil {
		fatalf(t, "lifecycle record: %v", err)
	}
	if want := "beforeAll\nTestFirst\nTestSecond\nafterAll\n"; string(got) != want {
		t.Errorf("lifecycle order:\n got  %q\n want %q", got, want)
	}
}

func (s *CanaryTestSuite) TestAnUncompilablePackageIsExit2(t *gotest.T) {
	s.check(t, "broken")
}

// A panic in a method fails the method and ends the test binary, so a
// sibling declared after it gets no verdict; the run is red regardless.
func (s *CanaryTestSuite) TestAPanicFailsTheMethod(t *gotest.T) {
	s.check(t, "panicking")
}

// Seeds replay as subtests of the fuzz wrapper on an ordinary run; a seed
// that fails fails its wrapper, and the census counts wrappers.
func (s *CanaryTestSuite) TestSeedsReplayWithVerdicts(t *gotest.T) {
	s.check(t, "fuzzing")
}

// streamOutput is what a -json stream printed for the test at path.
func streamOutput(stream, path string) string {
	var out strings.Builder
	for line := range strings.Lines(stream) {
		var ev struct{ Action, Test, Output string }
		if json.Unmarshal([]byte(line), &ev) == nil && ev.Test == path && ev.Action == "output" {
			out.WriteString(ev.Output)
		}
	}
	return out.String()
}

// Two suites share one package fixture; each runs in its own process with
// its own copy of the DAG, so each process must tear its copy down.
func (s *CanaryTestSuite) TestPackageFixtureTearsDownAfterEverySuite(t *gotest.T) {
	dir := s.check(t, "fixtureteardown").dir
	expectTeardowns(t, dir, 2)
}

// expectTeardowns checks that the fixture's AfterAll ran in want processes
// and that none left it up. Markers are keyed by pid.
func expectTeardowns(t *gotest.T, dir string, want int) {
	done, _ := filepath.Glob(filepath.Join(dir, "fixture-afterall-*"))
	if len(done) != want {
		t.Errorf("the package fixture's AfterAll ran in %d processes, want %d", len(done), want)
	}
	if alive, _ := filepath.Glob(filepath.Join(dir, "fixture-alive-*")); len(alive) != 0 {
		t.Errorf("the package fixture is still up in %d processes", len(alive))
	}
}

// A bench run selects the benchmark wrapper alone, and the fixture it uses
// must still be torn down when it finishes.
func (s *CanaryTestSuite) TestPackageFixtureTearsDownAfterBench(t *gotest.T) {
	dir := t.TempDir()
	code, names := s.bench(t, "fixtureteardown", dir)
	if code != 0 {
		t.Errorf("fixtureteardown bench: exit code %d, want 0", code)
	}
	if want := "BenchmarkFixtureAlive"; strings.Join(names, ",") != want {
		t.Errorf("fixtureteardown bench: results %v, want %v", names, want)
	}
	expectTeardowns(t, dir, 1)
}

// Seeds replay after the suite's Test function; a fixture the suite binds
// must still be up for them.
func (s *CanaryTestSuite) TestSeedsReplayAgainstLiveFixtures(t *gotest.T) {
	s.check(t, "fixturefuzzing")
}

// A shared fixture that fails to release what it holds fails the run after
// its last verdict; the -json stream must carry that failure, not only the
// exit code.
func (s *CanaryTestSuite) TestATeardownFailureReachesTheStream(t *gotest.T) {
	s.check(t, "teardownfailing")
}

// When one shared fixture fails to come up and another fails to release, the
// run has two failures and the stream must carry both: a leak reported only on
// stderr is a leak a -json consumer never sees.
func (s *CanaryTestSuite) TestATeardownFailureBesideASetupFailureReachesTheStream(t *gotest.T) {
	r := s.check(t, "teardownaftersetup")
	if !strings.Contains(r.stream, "would not release what it holds") {
		t.Errorf("teardownaftersetup: the stream carries no word of the teardown failure")
	}
	if !strings.Contains(r.stream, "would not come up") {
		t.Errorf("teardownaftersetup: the stream carries no word of the setup failure")
	}
}

// A shared fixture that fails to come up fails the suite that reads it and
// the run; a suite that reads none runs and reports. The -json stream must
// carry all three, not only the exit code.
func (s *CanaryTestSuite) TestASetupFailureReachesTheStream(t *gotest.T) {
	s.check(t, "setupfailing")
}

// A bench run waits for its fixtures before it dispatches; the same failure
// is a failed run there, not a command that broke, and it is the failure of
// the benchmarks that read the fixture alone: the one that reads none and
// the one that reads a fixture that came up both report a sample.
func (s *CanaryTestSuite) TestASetupFailureFailsABenchRun(t *gotest.T) {
	code, names := s.bench(t, "setupfailing", t.TempDir())
	if code != 1 {
		t.Errorf("setupfailing bench: exit code %d, want 1", code)
	}
	if want := "BenchmarkFixtureIsUp,BenchmarkRuns"; strings.Join(names, ",") != want {
		t.Errorf("setupfailing bench: results %v, want %v", names, want)
	}
}

// A shared fixture reached only through a chain of package fixtures is still
// started for the suite, and the suite reads its transferred state.
func (s *CanaryTestSuite) TestSharedFixturesReachThroughFixtureChains(t *gotest.T) {
	s.check(t, "fixturechain")
}

// A suite its guard skips sets up none of its fixtures, so one that would
// fail to come up leaves the run green.
func (s *CanaryTestSuite) TestAGuardRunsBeforeTheFixtures(t *gotest.T) {
	s.check(t, "guardskip")
}

// A goroutine BeforeAll starts with gotest.Go is waited for after AfterAll,
// which is what stops it; waiting earlier would deadlock the suite.
func (s *CanaryTestSuite) TestAGoroutineFromBeforeAllIsWaitedForAfterAfterAll(t *gotest.T) {
	// go test's own alarm bounds a deadlocked wait, which the runner's graceful
	// stop cannot end; a passing run never reaches it.
	r := s.runWith(t, "goroutines", t.TempDir(), nil, "-timeout=20s")
	if !slices.Contains(r.rows, "goroutines pass TestServerTestSuite") {
		t.Errorf("goroutines: the suite whose BeforeAll started a goroutine did not pass (exit %d): %v", r.code, r.rows)
	}
}

// Goroutines BeforeAll starts on Ts derived from the hook's (NewTWithDeadline,
// When, It, Each) are still waited for after AfterAll, which is what stops
// them.
func (s *CanaryTestSuite) TestADerivedTsGoroutineIsWaitedForAfterAfterAll(t *gotest.T) {
	// go test's own alarm bounds a deadlocked wait; a passing run never reaches it.
	r := s.runWith(t, "setupwaits", t.TempDir(), nil, "-timeout=20s", "-run=TestDerivedTestSuite")
	if !slices.Contains(r.rows, "setupwaits pass TestDerivedTestSuite") {
		t.Errorf("setupwaits: the suite whose BeforeAll started goroutines on derived Ts did not pass: %v", r.rows)
	}
}

// When the last of several goroutines BeforeAll started fails or panics, the
// suite fails and every other one is waited for before the fixture goes.
func (s *CanaryTestSuite) TestEveryGoroutineBeforeAllStartedIsWaitedFor(t *gotest.T) {
	dir := t.TempDir()
	r := s.runWith(t, "setupwaits", dir, nil, "-timeout=20s", "-run=TestFailLastTestSuite|TestPanicLastTestSuite")
	data, _ := os.ReadFile(filepath.Join(dir, testkit.MarkersFile))
	got := strings.Split(strings.TrimSpace(string(data)), "\n")
	for _, c := range []struct{ suite, prefix string }{
		{"TestFailLastTestSuite", "fail: "},
		{"TestPanicLastTestSuite", "panic: "},
	} {
		if !slices.Contains(r.rows, "setupwaits fail "+c.suite) {
			t.Errorf("setupwaits: %s did not fail: %v", c.suite, r.rows)
		}
		ended, released := slices.Index(got, c.prefix+"first goroutine ended"), slices.Index(got, c.prefix+"fixture.AfterAll")
		if ended < 0 || released < ended {
			t.Errorf("setupwaits: %s released its fixture before every goroutine ended: markers %v", c.suite, got)
		}
	}
}

// A goroutine started by gotest.Go that fails fatally fails the method at the
// wait: nothing after the wait runs.
func (s *CanaryTestSuite) TestAFatalGoroutineFailsTheMethodAtTheWait(t *gotest.T) {
	r := s.runWith(t, "goroutines", t.TempDir(), nil, "-timeout=20s")
	if !slices.Contains(r.rows, "goroutines fail TestGoexitTestSuite/TestGoexit") {
		t.Errorf("goroutines: the method whose goroutine failed did not fail: %v", r.rows)
	}
	if strings.Contains(r.stream, "the method went on after the wait") {
		t.Errorf("goroutines: the method went on after the wait on its failed goroutine")
	}
}

// A loop that breaks out of an Each table stops at the break: no later row
// runs, and the method goes on after the loop.
func (s *CanaryTestSuite) TestABreakOutOfEachStopsTheRows(t *gotest.T) {
	dir := s.check(t, "eachbreak").dir
	data, _ := os.ReadFile(filepath.Join(dir, testkit.MarkersFile))
	got := strings.Split(strings.TrimSpace(string(data)), "\n")
	if slices.ContainsFunc(got, func(m string) bool { return strings.HasPrefix(m, "row ") && m != "row first" }) {
		t.Errorf("eachbreak: a row after the break ran: %v", got)
	}
	if !slices.Contains(got, "row first") {
		t.Errorf("eachbreak: the first row did not run: %v", got)
	}
	if !slices.Contains(got, "after the loop") {
		t.Errorf("eachbreak: the method did not go on after the break: %v", got)
	}
}

// A fuzz execution whose BeforeEach fails or panics still runs its AfterEach,
// as a test method does.
func (s *CanaryTestSuite) TestAFuzzExecutionsAfterEachRunsWhenBeforeEachLeaves(t *gotest.T) {
	dir := s.check(t, "fuzzhooks").dir
	got, _ := os.ReadFile(filepath.Join(dir, testkit.MarkersFile))
	for _, want := range []string{"after broken", "after panic"} {
		if !strings.Contains(string(got), want+"\n") {
			t.Errorf("fuzzhooks: AfterEach did not run after a BeforeEach that left (%s missing):\n%s", want, got)
		}
	}
}

// What a shared fixture's setup prints to stdout is not the protocol: its
// state still reaches the suite whole.
func (s *CanaryTestSuite) TestASharedFixturesStdoutIsNotTheProtocol(t *gotest.T) {
	s.check(t, "noisysetup")
}

// Without --setup-timeout the wait for a shared fixture is the budget the
// fixture declared: a 2s Timeout taking 15s fails the suite reading it 2s
// into the setup, not 15. Timed from the setup's start, so the build of the
// setup program before it is not counted.
func (s *CanaryTestSuite) TestTheSetupWaitIsTheFixturesBudget(t *gotest.T) {
	r := s.check(t, "setupbudget")
	dir := r.dir
	markers, _ := testkit.ReadMarkers(filepath.Join(dir, testkit.MarkersFile))
	var began time.Time
	for _, m := range markers {
		if v, ok := strings.CutPrefix(m, "began="); ok {
			if n, err := strconv.ParseInt(v, 10, 64); err == nil {
				began = time.Unix(0, n)
			}
		}
	}
	failed := verdictTime(r.stream, "TestSlowTestSuite", "fail")
	if began.IsZero() || failed.IsZero() {
		fatalf(t, "setupbudget: no setup start (markers %v) or no fail verdict for TestSlowTestSuite in the stream", markers)
	}
	if took := failed.Sub(began); took > 10*time.Second {
		t.Errorf("setupbudget: the run took %s from the setup's start to the suite's verdict; the wait outlived the fixture's declared 2s budget", took)
	}
}

// verdictTime is the time the stream stamped on test's action event, zero
// when it has none.
func verdictTime(stream, test, action string) time.Time {
	for line := range strings.SplitSeq(stream, "\n") {
		var ev struct {
			Action string    `json:"Action"`
			Test   string    `json:"Test"`
			Time   time.Time `json:"Time"`
		}
		if json.Unmarshal([]byte(line), &ev) == nil && ev.Test == test && ev.Action == action {
			return ev.Time
		}
	}
	return time.Time{}
}

// A fuzz execution whose BeforeEach skips is skipped: its body never runs and
// its AfterEach still does.
func (s *CanaryTestSuite) TestAFuzzExecutionSkippedByBeforeEachStillTearsDown(t *gotest.T) {
	dir := s.check(t, "fuzzskiphook").dir
	got, _ := os.ReadFile(filepath.Join(dir, testkit.MarkersFile))
	if strings.Contains(string(got), "body ran\n") {
		t.Errorf("fuzzskiphook: the body ran after BeforeEach skipped:\n%s", got)
	}
	if !strings.Contains(string(got), "after skip\n") {
		t.Errorf("fuzzskiphook: AfterEach did not run after a BeforeEach that skipped:\n%s", got)
	}
}

// A goroutine started by gotest.Go that skips the test skips the method at
// the wait, without failing it.
func (s *CanaryTestSuite) TestAGoroutineThatSkipsSkipsTheMethod(t *gotest.T) {
	r := s.runWith(t, "goroutines", t.TempDir(), nil, "-timeout=20s", "-run=TestSkipTestSuite")
	if !slices.Contains(r.rows, "goroutines skip TestSkipTestSuite/TestSkip") {
		t.Errorf("goroutines: the method whose goroutine skipped was not skipped: %v", r.rows)
	}
}

// A goroutine started by gotest.Go that leaves through runtime.Goexit with
// no verdict fails the method, saying so.
func (s *CanaryTestSuite) TestAGoroutineLeavingWithoutAVerdictSaysSo(t *gotest.T) {
	r := s.runWith(t, "goroutines", t.TempDir(), nil, "-timeout=20s", "-run=TestRawGoexitTestSuite")
	if !slices.Contains(r.rows, "goroutines fail TestRawGoexitTestSuite/TestRawGoexit") {
		t.Errorf("goroutines: the method whose goroutine left through Goexit did not fail: %v", r.rows)
	}
	if !strings.Contains(r.stream, "goroutine started by gotest.Go exited via runtime.Goexit") {
		t.Errorf("goroutines: no failure says the goroutine exited via runtime.Goexit")
	}
}

// A BeforeAll taking a *testing.T that starts goroutines on gotest.NewT(t)
// and NewTWithDeadline(t) has them waited for after AfterAll.
func (s *CanaryTestSuite) TestATestingTHooksGoroutineIsWaitedForAfterAfterAll(t *gotest.T) {
	// go test's own alarm bounds a deadlocked wait; a passing run never reaches it.
	r := s.runWith(t, "setupwaits", t.TempDir(), nil, "-timeout=20s", "-run=TestStdTestSuite")
	if !slices.Contains(r.rows, "setupwaits pass TestStdTestSuite") {
		t.Errorf("setupwaits: the suite whose *testing.T BeforeAll started goroutines did not pass: %v", r.rows)
	}
}

// A goroutine started from one BeforeAll started, once AfterAll stopped it,
// is waited for too: its panic fails the suite.
func (s *CanaryTestSuite) TestAGoroutineStartedDuringTheWaitIsWaitedFor(t *gotest.T) {
	r := s.runWith(t, "setupwaits", t.TempDir(), nil, "-timeout=20s", "-run=TestLateTestSuite")
	if !slices.Contains(r.rows, "setupwaits fail TestLateTestSuite") || !strings.Contains(r.stream, "LATE-GOROUTINE-PANIC") {
		t.Errorf("setupwaits: the goroutine started during the wait was not waited for: %v", r.rows)
	}
}

// The wait on BeforeAll's goroutines is held to a declared SetupTimeout.
func (s *CanaryTestSuite) TestTheWaitOnSetupGoroutinesIsHeldToTheSetupTimeout(t *gotest.T) {
	r := s.runWith(t, "setupwaits", t.TempDir(), nil, "-timeout=20s", "-run=TestSlowTestSuite")
	if !slices.Contains(r.rows, "setupwaits fail TestSlowTestSuite") ||
		!strings.Contains(r.stream, "the wait on a goroutine BeforeAll started with gotest.Go exceeded its configured SetupTimeout") {
		t.Errorf("setupwaits: no SetupTimeout verdict names the wait on the slow goroutine: %v", r.rows)
	}
}

// A row that skips skips itself and the method; a row that ends the method's
// own T ends the method with that verdict alone.
func (s *CanaryTestSuite) TestEachRowsEndTheMethodWithTheirVerdict(t *gotest.T) {
	r := s.check(t, "eachskip")
	if strings.Contains(r.stream, "without a verdict") {
		t.Errorf("eachskip: a row that ended the method's own T was reported as leaving without a verdict")
	}
}

// A panic in BeforeEach is reported even when AfterEach then fails a fatal
// assertion, for a test method and for a fuzz execution.
func (s *CanaryTestSuite) TestAPanicAheadOfAFailingAfterEachIsReported(t *gotest.T) {
	r := s.runWith(t, "hookpanics", t.TempDir(), nil)
	for _, c := range []struct{ what, panic, mark string }{
		{"test method", "panic: TEST-BEFORE-EACH-PANIC", "MARK after the panic"},
		{"fuzz execution", "panic: FUZZ-BEFORE-EACH-PANIC", "MARK after the fuzz panic"},
	} {
		if !strings.Contains(r.stream, c.panic) {
			t.Errorf("hookpanics: the %s's panic was not reported", c.what)
		}
		if !strings.Contains(r.stream, c.mark) {
			t.Errorf("hookpanics: the %s's AfterEach did not run", c.what)
		}
	}
}

// The suite's Timeout is the whole method's budget: a fixture BeforeEach or
// AfterEach, or a suite BeforeEach, that overruns it fails the method,
// located at the hook and naming it, and nothing after a BeforeEach that
// overran starts but the AfterEach hooks. A hook waiting on its context sees
// it end, after the verdict.
func (s *CanaryTestSuite) TestAHookOverrunFailsTheMethod(t *gotest.T) {
	dir := t.TempDir()
	r := s.run(t, "hookoverrun", dir)
	if r.code != 1 {
		t.Errorf("hookoverrun: exit code %d, want 1", r.code)
	}
	for _, method := range []string{"TestOverrunTestSuite/TestSlow", "TestReleaseOverrunTestSuite/TestSlowRelease", "TestWaitingTestSuite/TestWaitedFor", "TestWaitingBeforeEachTestSuite/TestAfterTheWait", "TestLaterOverrunTestSuite/TestAfterTheSlowOne"} {
		if !slices.Contains(r.rows, "hookoverrun fail "+method) {
			t.Errorf("hookoverrun: %s did not fail: %v", method, r.rows)
		}
	}
	// fixture.go:30 and :41 are time.Sleep lines, fixture.go:53 and
	// hookoverrun_suite_test.go:55 the waitOn calls.
	for _, want := range []string{
		"fixture.go:30: SlowFixture.BeforeEach exceeded its configured Timeout",
		"fixture.go:41: SlowReleaseFixture.AfterEach exceeded its configured Timeout",
		"fixture.go:53: WaitingFixture.BeforeEach exceeded its configured Timeout",
		"hookoverrun_suite_test.go:55: BeforeEach exceeded its configured Timeout",
	} {
		if !strings.Contains(r.stream, want) {
			t.Errorf("hookoverrun: no %q in the stream", want)
		}
	}
	got, _ := testkit.ReadMarkers(filepath.Join(dir, testkit.MarkersFile))
	if strings.Contains(r.stream, "LaterFixture.BeforeEach failed") {
		t.Errorf("hookoverrun: the fixture after the one that overran was blamed too")
	}
	for _, never := range []string{"later fixture BeforeEach", "body", "suite BeforeEach", "body after the wait", "fixture gave up", "suite gave up"} {
		if slices.Contains(got, never) {
			t.Errorf("hookoverrun: %q ran after the budget was blown: %v", never, got)
		}
	}
	for _, want := range []string{"fixture saw its context end", "suite saw its context end"} {
		if !slices.Contains(got, want) {
			t.Errorf("hookoverrun: no %q in the markers: %v", want, got)
		}
	}
}

// A fixture BeforeEach that fails runs no AfterEach for that method: the
// hooks a method runs are the ones that paired with what ran. The failure is
// reported at the hook's own file, not inside the runtime's.
func (s *CanaryTestSuite) TestAFailedFixtureBeforeEachPairsWithNothing(t *gotest.T) {
	dir := t.TempDir()
	r := s.run(t, "fixturehookfail", dir)
	if r.code != 1 {
		t.Errorf("fixturehookfail: exit code %d, want 1", r.code)
	}
	got, _ := testkit.ReadMarkers(filepath.Join(dir, testkit.MarkersFile))
	want := []string{"fixture.BeforeAll", "fixture.BeforeEach", "fixture.BeforeEach", "second", "fixture.AfterEach", "fixture.AfterAll"}
	if strings.Join(got, ",") != strings.Join(want, ",") {
		t.Errorf("fixturehookfail: hooks ran %v, want %v", got, want)
	}
	if !strings.Contains(r.stream, "fixture.go:") {
		t.Errorf("fixturehookfail: the failure is not reported at the hook's file")
	}
	if strings.Contains(r.stream, "methods.go:") {
		t.Errorf("fixturehookfail: the failure is reported inside the runtime's file")
	}
}

// An error a fixture AfterEach returns is reported at the hook's own file,
// not inside the runtime's.
func (s *CanaryTestSuite) TestAFixtureAfterEachErrorIsReportedAtTheHook(t *gotest.T) {
	r := s.check(t, "fixtureafterfail")
	if !strings.Contains(r.stream, "fixture.go:") {
		t.Errorf("fixtureafterfail: the failure is not reported at the hook's file")
	}
	if strings.Contains(r.stream, "methods.go:") {
		t.Errorf("fixtureafterfail: the failure is reported inside the runtime's file")
	}
}

// A fixture with only an AfterEach runs it exactly when every fixture
// BeforeEach before it returned: ahead of the one that failed it runs,
// behind it it does not.
func (s *CanaryTestSuite) TestAnAfterEachOnlyFixtureRunsWhenItsTurnCame(t *gotest.T) {
	dir := t.TempDir()
	r := s.run(t, "fixtureorder", dir)
	if r.code != 1 {
		t.Errorf("fixtureorder: exit code %d, want 1", r.code)
	}
	for _, method := range []string{"TestAheadTestSuite/TestBody", "TestBehindTestSuite/TestBody"} {
		if !slices.Contains(r.rows, "fixtureorder fail "+method) {
			t.Errorf("fixtureorder: %s did not fail: %v", method, r.rows)
		}
	}
	got, _ := testkit.ReadMarkers(filepath.Join(dir, testkit.MarkersFile))
	if !slices.Contains(got, "ahead AfterEach") {
		t.Errorf("fixtureorder: the AfterEach ahead of the failed BeforeEach did not run: %v", got)
	}
	for _, never := range []string{"behind AfterEach", "ahead body", "behind body"} {
		if slices.Contains(got, never) {
			t.Errorf("fixtureorder: %q ran: %v", never, got)
		}
	}
}

// A failing fixture hook another package declares, a fixture's of that
// package or one promoted from its type, is located at its declaration:
// the base name by default, the full path under -fullpath.
func (s *CanaryTestSuite) TestAFixtureHookElsewhereIsLocatedAtIt(t *gotest.T) {
	full, err := filepath.Abs(filepath.Join("testdata", "fixtureelsewhere", "elsewhere", "elsewhere.go"))
	if err != nil {
		fatalf(t, "abs: %v", err)
	}
	full = filepath.ToSlash(full)
	for _, c := range []struct {
		flags []string
		file  string
	}{{nil, "elsewhere.go"}, {[]string{"-fullpath"}, full}} {
		r := s.runWith(t, "fixtureelsewhere", t.TempDir(), nil, c.flags...)
		if r.code != 1 {
			t.Errorf("fixtureelsewhere %v: exit code %d, want 1", c.flags, r.code)
		}
		// elsewhere.go:15 declares RemoteFixture.BeforeEach, :22 Hooks.AfterEach.
		for _, want := range []string{
			c.file + ":15: elsewhere_RemoteFixture.BeforeEach failed: the remote fixture is not ready",
			c.file + ":22: PromotedFixture.AfterEach failed: the embedded hook did not let go",
		} {
			if !strings.Contains(r.stream, " "+want+"\\n") {
				t.Errorf("fixtureelsewhere %v: no %q in the stream", c.flags, want)
			}
		}
	}
}

// A fixture whose BeforeAll failed still gets its AfterAll: whatever the setup
// reached is released, and the suite that binds it never opens.
func (s *CanaryTestSuite) TestAFixtureThatFailedToStartIsReleased(t *gotest.T) {
	dir := t.TempDir()
	r := s.run(t, "fixturesetupfail", dir)
	if r.code != 1 {
		t.Errorf("fixturesetupfail: exit code %d, want 1", r.code)
	}
	if slices.Contains(r.rows, "fixturesetupfail pass TestNeverOpensTestSuite/TestNever") {
		t.Errorf("fixturesetupfail: the suite ran without its fixture: %v", r.rows)
	}
	got, _ := testkit.ReadMarkers(filepath.Join(dir, testkit.MarkersFile))
	if want := []string{"fixture.BeforeAll", "fixture.AfterAll"}; !slices.Equal(got, want) {
		t.Errorf("fixturesetupfail: hooks ran %v, want %v", got, want)
	}
}

// A shared fixture whose setup failed is still released: what the attempt
// reached would otherwise leak, as it would not for a package fixture.
func (s *CanaryTestSuite) TestAFailedSharedSetupIsReleased(t *gotest.T) {
	dir := s.check(t, "failingshared").dir
	got, _ := testkit.ReadMarkers(filepath.Join(dir, testkit.MarkersFile))
	if !testkit.MarkerOrderOf(got, "leak.BeforeAll", "leak.AfterAll") {
		t.Errorf("failingshared: markers %v, want leak.BeforeAll then leak.AfterAll", got)
	}
}

// A fixture whose AfterAll fails between two setup attempts fails the run and
// reports that AfterAll's error, though its second attempt comes up: a package
// fixture and a shared one alike.
func (s *CanaryTestSuite) TestAFailedReleaseBetweenAttemptsFailsTheRun(t *gotest.T) {
	for _, c := range []struct{ pkg, says string }{
		{"packagerelease", "the package release leaked"},
		{"sharedrelease", "the shared release leaked"},
	} {
		r := s.check(t, c.pkg)
		if !strings.Contains(r.stream, c.says) {
			t.Errorf("%s: the run does not report the release's error %q", c.pkg, c.says)
		}
	}
}

// A shared fixture that fails its last attempt fails the run, though the suite
// binding it would pass without it.
func (s *CanaryTestSuite) TestAFailedLastAttemptFailsTheRun(t *gotest.T) {
	s.check(t, "lastattempt")
}

// A run stopped while a shared fixture's dependency sets up runs no AfterAll
// for the fixture, whose BeforeAll never ran, and the dependency's AfterAll.
func (s *CanaryTestSuite) TestASetupCancelledBeforeBeforeAllReleasesNothing(t *gotest.T) {
	got, out := s.interruptAt(t, "cancelbeforesetup", "hold.BeforeAll")
	if want := []string{"hold.BeforeAll", "hold.AfterAll"}; !slices.Equal(got, want) {
		t.Errorf("cancelbeforesetup: markers %v, want %v\n%s", got, want, out)
	}
}

// A run stopped while a shared fixture's AfterAll releases a failed attempt
// lets that AfterAll finish, starts no further attempt and releases once.
func (s *CanaryTestSuite) TestAStopDuringTheReleaseBetweenAttemptsLetsItFinish(t *gotest.T) {
	got, out := s.interruptAt(t, "releasestop", "retry.AfterAll")
	if !slices.Contains(got, "retry.AfterAll finished") {
		t.Errorf("releasestop: the stop cut the release short: markers %v\n%s", got, out)
	}
	if n := countOf(got, "retry.BeforeAll"); n != 1 {
		t.Errorf("releasestop: %d attempts, want 1: markers %v", n, got)
	}
	if n := countOf(got, "retry.AfterAll"); n != 1 {
		t.Errorf("releasestop: released %d times, want once: markers %v", n, got)
	}
}

// A package fixture whose release between attempts outlives its declared
// Timeout fails the run, though its second attempt comes up.
func (s *CanaryTestSuite) TestAReleaseThatOutlivesItsTimeoutFailsTheRun(t *gotest.T) {
	r := s.check(t, "releaseslow")
	if !strings.Contains(r.stream, "exceeded its configured Timeout of 200ms") {
		t.Errorf("releaseslow: the run does not report the release's overrun")
	}
}

// A shared fixture whose Hydrate fails partway still gets its Dehydrate.
func (s *CanaryTestSuite) TestAFailedHydrateIsDehydrated(t *gotest.T) {
	dir := s.check(t, "hydratefail").dir
	got, _ := testkit.ReadMarkers(filepath.Join(dir, testkit.MarkersFile))
	if want := []string{"pool.Hydrate", "pool.Dehydrate"}; !slices.Equal(got, want) {
		t.Errorf("hydratefail: markers %v, want %v", got, want)
	}
}

// interruptAt runs pkg, interrupts the run once marker is stamped and returns
// the markers and output the run left.
func (s *canary) interruptAt(t *gotest.T, pkg, marker string) ([]string, string) {
	dir := t.TempDir()
	cmd := exec.Command(s.binary, "-json", "./testdata/"+pkg+"/") //nolint:gosec // G204: controlled binary with fixed args
	cmd.Env = childEnv(dir)
	var out bytes.Buffer
	cmd.Stdout, cmd.Stderr = &out, &out
	cmd.WaitDelay = 15 * time.Second
	tree, err := testkit.StartCLI(cmd)
	if err != nil {
		fatalf(t, "start: %v", err)
	}
	markers := filepath.Join(dir, testkit.MarkersFile)
	for deadline := time.Now().Add(90 * time.Second); ; time.Sleep(20 * time.Millisecond) {
		got, _ := testkit.ReadMarkers(markers)
		if slices.Contains(got, marker) {
			break
		}
		if time.Now().After(deadline) {
			_ = tree.Kill()
			_ = cmd.Wait()
			tree.Release()
			fatalf(t, "%s: the run never reached %s:\n%s", pkg, marker, out.String())
		}
	}
	if err := testkit.Interrupt(cmd, tree); err != nil {
		_ = tree.Kill()
		_ = cmd.Wait()
		tree.Release()
		fatalf(t, "%s: signal: %v", pkg, err)
	}
	_ = cmd.Wait()
	tree.Release()
	got, _ := testkit.ReadMarkers(markers)
	return got, out.String()
}

func countOf(events []string, event string) int {
	n := 0
	for _, e := range events {
		if e == event {
			n++
		}
	}
	return n
}

// A suite binding a shared fixture has no state under plain go test: it fails
// naming the fixture and the runner, before any method runs.
func (s *CanaryTestSuite) TestASharedFixtureSuiteRefusesPlainGoTest(t *gotest.T) {
	root, err := filepath.Abs("../..")
	if err != nil {
		fatalf(t, "abs: %v", err)
	}
	dir := t.TempDir()
	if _, err := testkit.StagePackage(root, filepath.Join("testdata", "fixturechain"), dir, "example.com/plain"); err != nil {
		fatalf(t, "stage fixturechain: %v", err)
	}
	generate := exec.Command(s.binary, "generate", "./fixturechain/") //nolint:gosec // G204: controlled binary with fixed args
	generate.Dir = dir
	generate.Env = childEnv(t.TempDir())
	if out, err := generate.CombinedOutput(); err != nil {
		fatalf(t, "generate fixturechain: %v\n%s", err, out)
	}
	run := exec.Command("go", "test", "-count=1", "-v", "./fixturechain/")
	run.Dir = dir
	run.Env = childEnv(t.TempDir())
	raw, err := run.CombinedOutput()
	out := string(raw)
	code := 0
	if err != nil {
		var exitErr *exec.ExitError
		if !errors.As(err, &exitErr) {
			fatalf(t, "go test fixturechain: %v", err)
		}
		code = exitErr.ExitCode()
	}
	if code != 1 {
		t.Errorf("plain go test on a shared-fixture suite: exit code %d, want 1\n%s", code, out)
	}
	if !strings.Contains(out, "suite binds shared fixture TokenSharedFixture, whose state only the gotest runner provides; run via go tool gotest") {
		t.Errorf("plain go test on a shared-fixture suite: the suite did not name the fixture it binds\n%s", out)
	}
	if strings.Contains(out, "=== RUN   TestChainTestSuite/") {
		t.Errorf("plain go test on a shared-fixture suite: a method ran without the fixture's state\n%s", out)
	}
}

// A benchmark that panics fails instead of being lost: the run is red, the
// benchmark after it still reports, and AfterAll runs before the fixture is
// released.
func (s *CanaryTestSuite) TestAPanickingBenchmarkStillTearsDown(t *gotest.T) {
	dir := t.TempDir()
	code, names := s.bench(t, "benchpanic", dir)
	if code != 1 {
		t.Errorf("benchpanic: exit code %d, want 1", code)
	}
	if want := "BenchmarkStillRuns"; strings.Join(names, ",") != want {
		t.Errorf("benchpanic: results %v, want [%s]", names, want)
	}
	got, _ := testkit.ReadMarkers(filepath.Join(dir, testkit.MarkersFile))
	if !testkit.MarkerOrderOf(got, "panics", "suite.AfterAll", "fixture.AfterAll") {
		t.Errorf("benchpanic: markers %v, want panics, suite.AfterAll, fixture.AfterAll in order", got)
	}
}

// After the first failure a FailFast suite reports the methods it did not
// run: each is skipped, naming the method that stopped the suite.
func (s *CanaryTestSuite) TestFailFastReportsWhatItSkipped(t *gotest.T) {
	r := s.run(t, "failfast", t.TempDir())
	if r.code != 1 {
		t.Errorf("failfast: exit code %d, want 1", r.code)
	}
	for _, want := range []string{"failfast fail TestFailFastTestSuite/TestFirst", "failfast skip TestFailFastTestSuite/TestSecond", "failfast skip TestFailFastTestSuite/TestThird"} {
		if !slices.Contains(r.rows, want) {
			t.Errorf("failfast: missing %q in %v", want, r.rows)
		}
	}
	if !strings.Contains(r.stream, "FailFast: TestFirst") {
		t.Errorf("failfast: the skipped methods do not name TestFirst as the reason")
	}
	if strings.Contains(r.stream, "methods.go:") {
		t.Errorf("failfast: a skip names a runtime file")
	}
}

// A parallel method that fails only in its cleanup still stops the methods
// of a FailFast suite that have not started.
func (s *CanaryTestSuite) TestFailFastSeesACleanupFailure(t *gotest.T) {
	r := s.runWith(t, "failfastcleanup", t.TempDir(), nil, "-parallel=1")
	var failed, skipped int
	for _, row := range r.rows {
		switch {
		case strings.HasPrefix(row, "failfastcleanup fail TestCleanupFailFastTestSuite/"):
			failed++
		case strings.HasPrefix(row, "failfastcleanup skip TestCleanupFailFastTestSuite/"):
			skipped++
		}
	}
	if failed != 1 || skipped != 1 {
		t.Errorf("failfastcleanup: %d methods failed and %d were skipped, want 1 and 1: %v", failed, skipped, r.rows)
	}
}

// FailFast reads each method's own verdict: a failure BeforeAll recorded
// without stopping does not stop the methods.
func (s *CanaryTestSuite) TestFailFastIgnoresASoftSuiteFailure(t *gotest.T) {
	r := s.run(t, "failfastsoft", t.TempDir())
	for _, want := range []string{"failfastsoft pass TestSoftFailFastTestSuite/TestFirst", "failfastsoft pass TestSoftFailFastTestSuite/TestSecond"} {
		if !slices.Contains(r.rows, want) {
			t.Errorf("failfastsoft: missing %q in %v", want, r.rows)
		}
	}
}

// A panic in a parallel method ends the suite as go test would: AfterAll and
// the fixture teardown run while a sibling is still mid-flight, and that
// sibling's AfterEach never runs.
func (s *CanaryTestSuite) TestAParallelPanicEndsTheSuiteAroundItsSiblings(t *gotest.T) {
	dir := t.TempDir()
	r := s.run(t, "parallelpanic", dir)
	if r.code != 1 || !slices.Contains(r.rows, "parallelpanic fail TestParallelPanicTestSuite/TestPanics") {
		t.Errorf("parallelpanic: the panicking method did not fail the run (exit %d): %v", r.code, r.rows)
	}
	got, _ := testkit.ReadMarkers(filepath.Join(dir, testkit.MarkersFile))
	for _, want := range []string{"slow started", "afterall", "fixture down"} {
		if !slices.Contains(got, want) {
			t.Errorf("parallelpanic: %q missing from the markers: %v", want, got)
		}
	}
	if slices.Contains(got, "aftereach TestParallelPanicTestSuite/TestSlow") || slices.Contains(got, "slow finished") {
		t.Errorf("parallelpanic: the sibling of a panicking method ran to its end or its AfterEach ran: %v", got)
	}
}

// benchReport is the part of a bench --json report the canary reads.
type benchReport struct {
	Baseline struct {
		GOMAXPROCS int `json:"gomaxprocs"`
		Results    []struct {
			Name    string `json:"name"`
			Samples []struct {
				Metrics map[string]float64 `json:"metrics"`
			} `json:"samples"`
		} `json:"results"`
	} `json:"baseline"`
	Missing []string `json:"missing"`
	Added   []string `json:"added"`
}

// bench runs a capturing bench run over one fixture package and returns its
// exit code and the benchmark names the report carries a sample for.
func (s *canary) bench(t *gotest.T, pkg, canaryDir string) (int, []string) {
	code, report := s.benchReport(t, pkg, canaryDir)
	names := make([]string, 0, len(report.Baseline.Results))
	for _, r := range report.Baseline.Results {
		names = append(names, r.Name)
	}
	sort.Strings(names)
	return code, names
}

// benchReport runs a capturing bench run over one fixture package with extra
// gotest flags and returns its exit code, the parsed report and stderr.
func (s *canary) benchReport(t *gotest.T, pkg, canaryDir string, extra ...string) (int, benchReport) {
	code, stdout, stderr := s.benchRaw(t, pkg, canaryDir, extra...)
	var report benchReport
	if err := json.Unmarshal(stdout, &report); err != nil {
		fatalf(t, "bench %s: report is not the JSON document bench --json promises: %v\nstdout:\n%s\nstderr:\n%s", pkg, err, stdout, stderr)
	}
	return code, report
}

// benchRaw runs bench --json over one fixture package and returns the exit
// code and both streams as they were.
func (s *canary) benchRaw(t *gotest.T, pkg, canaryDir string, extra ...string) (int, []byte, []byte) {
	args := append([]string{"bench", "--json"}, extra...)
	args = append(args, "./testdata/"+pkg+"/", "-benchtime=1x")
	return s.invoke(t, canaryDir, args...)
}

// invoke runs the CLI with the arguments as given and returns the exit code
// and both streams as they were.
func (s *canary) invoke(t *gotest.T, canaryDir string, args ...string) (int, []byte, []byte) {
	cmd := exec.Command(s.binary, args...) //nolint:gosec // G204: controlled binary with fixed args
	cmd.Env = childEnv(canaryDir)
	var stdout, stderr bytes.Buffer
	cmd.Stdout, cmd.Stderr = &stdout, &stderr
	code := 0
	if err := cmd.Run(); err != nil {
		var exitErr *exec.ExitError
		if !errors.As(err, &exitErr) {
			fatalf(t, "run %v: %v", args, err)
		}
		code = exitErr.ExitCode()
	}
	return code, stdout.Bytes(), stderr.Bytes()
}

// A gate of zero is no gate: the run compares and is not held to the sample
// count a gate needs.
func (s *CanaryTestSuite) TestAGateOfZeroIsNoGate(t *gotest.T) {
	dir := t.TempDir()
	baseline := foreignBaseline(t, dir)
	code, _, stderr := s.benchRaw(t, "benching", dir, "--against="+baseline, "--gate=0")
	if code != 0 {
		t.Errorf("--gate=0 without -count: exit code %d, want 0\n%s", code, stderr)
	}
}

// What follows -args is the test binary's; the -v a bench run adds for itself
// goes in front of it.
func (s *CanaryTestSuite) TestABenchRunKeepsTheBinaryWordsLast(t *gotest.T) {
	dir := t.TempDir()
	code, _, stderr := s.invoke(t, dir, "bench", "./testdata/benching/", "-args", "-test.benchtime=1x")
	if code != 0 {
		t.Errorf("bench with -args: exit code %d, want 0\n%s", code, stderr)
	}
}

// A gate judges the benchmarks the baseline and the run share. A baseline
// from elsewhere shares none: the gate fails rather than passing on nothing,
// and the report names what each side has that the other lacks.
func (s *CanaryTestSuite) TestAGateWithNothingToCompareFails(t *gotest.T) {
	dir := t.TempDir()
	baseline := foreignBaseline(t, dir)
	code, report := s.benchReport(t, "benching", dir, "--against="+baseline, "--gate=10", "-count=4")
	if code != 1 {
		t.Errorf("gate against a foreign baseline: exit code %d, want 1", code)
	}
	if want := "example.com/elsewhere OtherTestSuite/BenchmarkGone"; strings.Join(report.Missing, ",") != want {
		t.Errorf("missing %v, want [%s]", report.Missing, want)
	}
	if len(report.Added) != 6 {
		t.Errorf("added %v, want the six benchmark leaves of the run", report.Added)
	}
}

// foreignBaseline writes a baseline whose only benchmark lives in a package
// this run does not have.
func foreignBaseline(t *gotest.T, dir string) string {
	baseline := filepath.Join(dir, "baseline.json")
	foreign := `{"schemaVersion":1,"goVersion":"` + runtime.Version() + `","goos":"` + runtime.GOOS + `","goarch":"` + runtime.GOARCH + `",` +
		`"results":[{"package":"example.com/elsewhere","suite":"OtherTestSuite","name":"BenchmarkGone","samples":[{"nsPerOp":1}]}]}`
	if err := os.WriteFile(baseline, []byte(foreign), 0o600); err != nil {
		fatalf(t, "write baseline: %v", err)
	}
	return baseline
}

// A gate judges by a significance test, which needs four samples a side;
// below -count=4 it refuses instead of guessing.
func (s *CanaryTestSuite) TestAGateNeedsSamples(t *gotest.T) {
	dir := t.TempDir()
	baseline := foreignBaseline(t, dir)
	code, _, stderr := s.benchRaw(t, "benching", dir, "--against="+baseline, "--gate=10")
	if code != 2 {
		t.Errorf("gate without -count: exit code %d, want 2", code)
	}
	if !strings.Contains(string(stderr), "-count=4 or more") {
		t.Errorf("gate without -count: stderr does not name the requirement:\n%s", stderr)
	}
	// Four is enough: the gate then judges, and this baseline shares nothing.
	code, _, _ = s.benchRaw(t, "benching", dir, "--against="+baseline, "--gate=10", "-count=4")
	if code != 1 {
		t.Errorf("gate with -count=4: exit code %d, want 1", code)
	}
}

// Saving the run to the file it is compared against would compare the run
// with itself; the command refuses before anything runs.
func (s *CanaryTestSuite) TestSaveAndAgainstOnOneFileIsRefused(t *gotest.T) {
	dir := t.TempDir()
	baseline := filepath.Join(dir, "baseline.json")
	if err := os.WriteFile(baseline, []byte(`{"schemaVersion":1}`), 0o600); err != nil {
		fatalf(t, "write baseline: %v", err)
	}
	code, _, stderr := s.benchRaw(t, "benching", dir, "--save="+baseline, "--against="+baseline)
	if code != 2 {
		t.Errorf("--save and --against on one file: exit code %d, want 2", code)
	}
	if !strings.Contains(string(stderr), "same file") {
		t.Errorf("--save and --against on one file: stderr does not say so:\n%s", stderr)
	}
	if data, err := os.ReadFile(baseline); err != nil || string(data) != `{"schemaVersion":1}` {
		t.Errorf("the baseline was overwritten: %s", data)
	}
}

// A green bench run reports a sample for every declared benchmark and exits
// 0; a missing one is the bench census's job. A method that opens
// sub-benchmarks is reported through its leaves, keyed by their path below
// the suite; a metric a benchmark reports is kept on its sample; the run's
// GOMAXPROCS is recorded.
func (s *CanaryTestSuite) TestEveryBenchmarkReportsAResult(t *gotest.T) {
	code, report := s.benchReport(t, "benching", t.TempDir())
	if code != 0 {
		t.Errorf("benching: exit code %d, want 0", code)
	}
	names := make([]string, 0, len(report.Baseline.Results))
	for _, r := range report.Baseline.Results {
		names = append(names, r.Name)
		if r.Name == "BenchmarkMetric" && (len(r.Samples) == 0 || r.Samples[0].Metrics["widgets/op"] != 4.5) {
			t.Errorf("BenchmarkMetric: samples %v, want widgets/op 4.5", r.Samples)
		}
	}
	sort.Strings(names)
	want := []string{"BenchmarkContext", "BenchmarkFirst", "BenchmarkMetric", "BenchmarkSecond", "BenchmarkSplit/long", "BenchmarkSplit/short"}
	if strings.Join(names, ",") != strings.Join(want, ",") {
		t.Errorf("benching: results %v, want %v", names, want)
	}
	if report.Baseline.GOMAXPROCS == 0 {
		t.Errorf("benching: the baseline records no GOMAXPROCS")
	}
}

// A benchmark that fails, halts or skips reports no sample and turns the run
// red; FailNow stops the method where it is called.
func (s *CanaryTestSuite) TestFailingBenchmarksReportNoResult(t *gotest.T) {
	dir := t.TempDir()
	code, names := s.bench(t, "benchfailing", dir)
	if code != 1 {
		t.Errorf("benchfailing: exit code %d, want 1", code)
	}
	if len(names) != 0 {
		t.Errorf("benchfailing: results %v, want none", names)
	}
	if _, err := os.Stat(filepath.Join(dir, "after-failnow")); err == nil {
		t.Errorf("the statement after FailNow ran: the benchmark did not halt")
	}
}
