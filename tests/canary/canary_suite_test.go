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

	"github.com/mvrahden/go-test/internal/testkit"
	"github.com/mvrahden/go-test/pkg/gotest"
)

// CanaryTestSuite runs one fixture package per method.
// Sequential: it builds the CLI once and each method spawns a full pipeline run.
type CanaryTestSuite struct {
	binary     string
	expected   map[string]expectation
	lastStream string // the -json stdout of the last run, for checks on what it carried
}

type expectation struct {
	exit int
	rows []string
}

func fatalf(t *gotest.T, format string, args ...any) {
	t.Errorf(format, args...)
	t.FailNow()
}

func (s *CanaryTestSuite) BeforeAll(t *gotest.T) {
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

type event struct {
	Action  string `json:"Action"`
	Package string `json:"Package"`
	Test    string `json:"Test"`
}

// run executes the CLI in -json mode over one fixture package and returns
// its exit code and the verdict rows it produced, in golden form.
func (s *CanaryTestSuite) run(t *gotest.T, pkg, canaryDir string, extra ...string) (int, []string) {
	return s.runEnv(t, pkg, canaryDir, nil, extra...)
}

// runEnv is run with extra variables in the child's environment.
func (s *CanaryTestSuite) runEnv(t *gotest.T, pkg, canaryDir string, env []string, extra ...string) (int, []string) {
	cmd := exec.Command(s.binary, append(append([]string{"-json"}, extra...), "./testdata/"+pkg+"/")...) //nolint:gosec // G204: controlled binary with fixed args
	cmd.Env = append(childEnv(canaryDir), env...)
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
	s.lastStream = stdout.String()
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
	return code, rows
}

// childEnv is the fixture process's environment: the canary dir, and never
// the snapshot update switch. A refresh of the repository's own snapshots
// must not rewrite the snapshotting fixture's deliberately stale baseline.
func childEnv(canaryDir string) []string {
	env := make([]string, 0, len(os.Environ())+1)
	for _, kv := range os.Environ() {
		if strings.HasPrefix(kv, "GOTEST_UPDATE_SNAPSHOTS=") {
			continue
		}
		env = append(env, kv)
	}
	return append(env, "GOTEST_CANARY_DIR="+canaryDir)
}

// check runs a fixture and compares exit code and rows with the golden.
func (s *CanaryTestSuite) check(t *gotest.T, pkg string) string {
	want, ok := s.expected[pkg]
	if !ok {
		fatalf(t, "no golden entry for %s", pkg)
	}
	dir := t.TempDir()
	code, rows := s.run(t, pkg, dir)
	if code != want.exit {
		t.Errorf("%s: exit code %d, want %d", pkg, code, want.exit)
	}
	if strings.Join(rows, "\n") != strings.Join(want.rows, "\n") {
		t.Errorf("%s: verdicts differ from the golden list\n got:\n  %s\n want:\n  %s",
			pkg, strings.Join(rows, "\n  "), strings.Join(want.rows, "\n  "))
	}
	return dir
}

func (s *CanaryTestSuite) TestPassingSuiteIsGreen(t *gotest.T) {
	s.check(t, "passing")
}

func (s *CanaryTestSuite) TestEveryAssertionFamilyFails(t *gotest.T) {
	s.check(t, "failing")
}

// A baseline the value no longer matches must fail; MatchSnapshot comparing
// nothing would leave every golden check in the repository silent.
func (s *CanaryTestSuite) TestASnapshotMismatchFails(t *gotest.T) {
	s.check(t, "snapshotting")
}

// An Exclusive suite is dispatched strictly alone, after every non-exclusive
// suite has drained (AGENTS.md). The fixture's three suites stamp their
// windows; the exclusive one must open after both bulk windows closed.
func (s *CanaryTestSuite) TestAnExclusiveSuiteRunsAfterTheBulk(t *gotest.T) {
	dir := s.check(t, "exclusive")
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
	dir := s.check(t, "failnow")
	if _, err := os.Stat(filepath.Join(dir, "after-failnow")); err == nil {
		t.Errorf("the statement after a failed assertion ran: the assertion did not halt")
	}
}

func (s *CanaryTestSuite) TestAfterEachRunsWhenTheTestFailed(t *gotest.T) {
	dir := s.check(t, "aftereach")
	if _, err := os.Stat(filepath.Join(dir, "aftereach-ran")); err != nil {
		t.Errorf("AfterEach did not run after a failing test: %v", err)
	}
}

func (s *CanaryTestSuite) TestLifecycleRunsInOrder(t *gotest.T) {
	dir := s.check(t, "lifecycle")
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

// An async method completes when done() is called from any goroutine; one
// that never calls it fails at the suite's deadline instead of hanging.
func (s *CanaryTestSuite) TestAsyncMethodsWaitForDone(t *gotest.T) {
	s.check(t, "asynchronous")
}

// Two suites share one package fixture; each runs in its own process with
// its own copy of the DAG, so each process must tear its copy down.
func (s *CanaryTestSuite) TestPackageFixtureTearsDownAfterEverySuite(t *gotest.T) {
	dir := s.check(t, "fixtureteardown")
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
	s.check(t, "teardownaftersetup")
	if !strings.Contains(s.lastStream, "would not release what it holds") {
		t.Errorf("teardownaftersetup: the stream carries no word of the teardown failure")
	}
	if !strings.Contains(s.lastStream, "would not come up") {
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

// A fixture BeforeEach that fails registered no AfterEach for that method:
// the hooks a method runs are the ones that paired with what ran. The
// failure is reported at the hook's own file, not inside the runtime's.
func (s *CanaryTestSuite) TestAFailedFixtureBeforeEachPairsWithNothing(t *gotest.T) {
	dir := t.TempDir()
	code, _ := s.run(t, "fixturehookfail", dir)
	if code != 1 {
		t.Errorf("fixturehookfail: exit code %d, want 1", code)
	}
	got, _ := testkit.ReadMarkers(filepath.Join(dir, testkit.MarkersFile))
	want := []string{"fixture.BeforeAll", "fixture.BeforeEach", "fixture.BeforeEach", "second", "fixture.AfterEach", "fixture.AfterAll"}
	if strings.Join(got, ",") != strings.Join(want, ",") {
		t.Errorf("fixturehookfail: hooks ran %v, want %v", got, want)
	}
	if !strings.Contains(s.lastStream, "fixture.go:") {
		t.Errorf("fixturehookfail: the failure is not reported at the hook's file")
	}
	if strings.Contains(s.lastStream, "methods.go:") {
		t.Errorf("fixturehookfail: the failure is reported inside the runtime's file")
	}
}

// The suite's Timeout is the whole method's budget: a fixture BeforeEach that
// sleeps past it fails the method, and the body never starts with an expired
// context.
func (s *CanaryTestSuite) TestAHookOverrunFailsTheMethod(t *gotest.T) {
	dir := t.TempDir()
	code, rows := s.run(t, "hookoverrun", dir)
	if code != 1 {
		t.Errorf("hookoverrun: exit code %d, want 1", code)
	}
	if !slices.Contains(rows, "hookoverrun fail TestOverrunTestSuite/TestSlow") {
		t.Errorf("hookoverrun: TestSlow did not fail: %v", rows)
	}
	if got, _ := testkit.ReadMarkers(filepath.Join(dir, testkit.MarkersFile)); slices.Contains(got, "body") {
		t.Errorf("hookoverrun: the body ran after the budget was blown: %v", got)
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
	code, rows := s.run(t, "failfast", t.TempDir())
	if code != 1 {
		t.Errorf("failfast: exit code %d, want 1", code)
	}
	for _, want := range []string{"failfast fail TestFailFastTestSuite/TestFirst", "failfast skip TestFailFastTestSuite/TestSecond", "failfast skip TestFailFastTestSuite/TestThird"} {
		if !slices.Contains(rows, want) {
			t.Errorf("failfast: missing %q in %v", want, rows)
		}
	}
	if !strings.Contains(s.lastStream, "FailFast: TestFirst") {
		t.Errorf("failfast: the skipped methods do not name TestFirst as the reason")
	}
}

// FailFast reads each method's own verdict: a failure BeforeAll recorded
// without stopping does not stop the methods.
func (s *CanaryTestSuite) TestFailFastIgnoresASoftSuiteFailure(t *gotest.T) {
	_, rows := s.run(t, "failfastsoft", t.TempDir())
	for _, want := range []string{"failfastsoft pass TestSoftFailFastTestSuite/TestFirst", "failfastsoft pass TestSoftFailFastTestSuite/TestSecond"} {
		if !slices.Contains(rows, want) {
			t.Errorf("failfastsoft: missing %q in %v", want, rows)
		}
	}
}

// A fixture whose BeforeAll failed still gets its AfterAll: whatever the setup
// reached is released, and the suite that binds it never opens.
func (s *CanaryTestSuite) TestAFixtureThatFailedToStartIsReleased(t *gotest.T) {
	dir := t.TempDir()
	code, rows := s.run(t, "fixturesetupfail", dir)
	if code != 1 {
		t.Errorf("fixturesetupfail: exit code %d, want 1", code)
	}
	if slices.Contains(rows, "fixturesetupfail pass TestNeverOpensTestSuite/TestNever") {
		t.Errorf("fixturesetupfail: the suite ran without its fixture: %v", rows)
	}
	got, _ := testkit.ReadMarkers(filepath.Join(dir, testkit.MarkersFile))
	if want := []string{"fixture.BeforeAll", "fixture.AfterAll"}; strings.Join(got, ",") != strings.Join(want, ",") {
		t.Errorf("fixturesetupfail: hooks ran %v, want %v", got, want)
	}
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
	if !strings.Contains(out, "binds shared fixture TokenSharedFixture; run via go tool gotest") {
		t.Errorf("plain go test on a shared-fixture suite: the suite did not name the fixture it binds\n%s", out)
	}
	if strings.Contains(out, "=== RUN   TestChainTestSuite/") {
		t.Errorf("plain go test on a shared-fixture suite: a method ran without the fixture's state\n%s", out)
	}
}

// A goroutine BeforeAll starts with gotest.Go is waited for after AfterAll,
// which is what stops it; waiting earlier would deadlock the suite.
func (s *CanaryTestSuite) TestAGoroutineFromBeforeAllIsWaitedForAfterAfterAll(t *gotest.T) {
	// go test's own alarm bounds a deadlocked wait, which the runner's graceful
	// stop cannot end; a passing run never reaches it.
	code, rows := s.run(t, "goroutines", t.TempDir(), "-timeout=20s")
	if !slices.Contains(rows, "goroutines pass TestServerTestSuite") {
		t.Errorf("goroutines: the suite whose BeforeAll started a goroutine did not pass (exit %d): %v", code, rows)
	}
}

// A goroutine started by gotest.Go that fails fatally fails the method at the
// wait: nothing after the wait runs.
func (s *CanaryTestSuite) TestAFatalGoroutineFailsTheMethodAtTheWait(t *gotest.T) {
	_, rows := s.run(t, "goroutines", t.TempDir(), "-timeout=20s")
	if !slices.Contains(rows, "goroutines fail TestGoexitTestSuite/TestGoexit") {
		t.Errorf("goroutines: the method whose goroutine failed did not fail: %v", rows)
	}
	if strings.Contains(s.lastStream, "the method went on after the wait") {
		t.Errorf("goroutines: the method went on after the wait on its failed goroutine")
	}
}

// A fuzz execution whose BeforeEach fails or panics still runs its AfterEach,
// as a test method does.
func (s *CanaryTestSuite) TestAFuzzExecutionsAfterEachRunsWhenBeforeEachLeaves(t *gotest.T) {
	dir := s.check(t, "fuzzhooks")
	got, _ := os.ReadFile(filepath.Join(dir, "markers"))
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
// fixture declared: a 2s Timeout taking 15s fails the run in 2s, not 15.
func (s *CanaryTestSuite) TestTheSetupWaitIsTheFixturesBudget(t *gotest.T) {
	began := time.Now()
	s.check(t, "setupbudget")
	if took := time.Since(began); took > 10*time.Second {
		t.Errorf("setupbudget: the run took %s; the wait outlived the fixture's declared 2s budget", took)
	}
}

// A shared fixture bound by one suite alone never starts when -skip names
// that suite: nothing dispatched requires it.
func (s *CanaryTestSuite) TestASkippedSuitesFixtureNeverStarts(t *gotest.T) {
	dir := t.TempDir()
	code, rows := s.run(t, "skipped", dir, "-skip=TestBoundTestSuite")
	if code != 0 {
		t.Errorf("skipped: exit code %d, want 0: %v", code, rows)
	}
	if !slices.Contains(rows, "skipped pass TestFreeTestSuite/TestOne") {
		t.Errorf("skipped: the free suite did not pass: %v", rows)
	}
	if got, _ := os.ReadFile(filepath.Join(dir, "markers")); strings.Contains(string(got), "shared up") {
		t.Errorf("skipped: the shared fixture started although -skip left no suite needing it")
	}
	// A -skip naming one method leaves the suite running: its fixture starts.
	dir = t.TempDir()
	code, rows = s.run(t, "skipped", dir, "-skip=TestBoundTestSuite/TestOne")
	if code != 0 || !slices.Contains(rows, "skipped pass TestBoundTestSuite/TestTwo") {
		t.Errorf("skipped: -skip of one method did not leave the rest of the suite passing (exit %d): %v", code, rows)
	}
	if got, _ := os.ReadFile(filepath.Join(dir, "markers")); !strings.Contains(string(got), "shared up") {
		t.Errorf("skipped: -skip of one method kept the suite's fixture from starting")
	}
}

// The runner cannot read a suite's guard, so a guard-skipped suite's shared
// fixture is started for it; when it fails, the run and the suite fail, the
// guard never having run.
func (s *CanaryTestSuite) TestAGuardedSuitesSharedFixtureStillFailsTheRun(t *gotest.T) {
	s.check(t, "guardedshared")
}

// A panic in a parallel method ends the suite as go test would: AfterAll and
// the fixture teardown run while a sibling is still mid-flight, and that
// sibling's AfterEach never runs.
func (s *CanaryTestSuite) TestAParallelPanicEndsTheSuiteAroundItsSiblings(t *gotest.T) {
	dir := t.TempDir()
	code, rows := s.run(t, "parallelpanic", dir)
	if code != 1 || !slices.Contains(rows, "parallelpanic fail TestParallelPanicTestSuite/TestPanics") {
		t.Errorf("parallelpanic: the panicking method did not fail the run (exit %d): %v", code, rows)
	}
	got, _ := os.ReadFile(filepath.Join(dir, "markers"))
	markers := string(got)
	for _, want := range []string{"slow started\n", "afterall\n", "fixture down\n"} {
		if !strings.Contains(markers, want) {
			t.Errorf("parallelpanic: %q missing from the markers:\n%s", strings.TrimSpace(want), markers)
		}
	}
	if strings.Contains(markers, "aftereach TestParallelPanicTestSuite/TestSlow") || strings.Contains(markers, "slow finished") {
		t.Errorf("parallelpanic: the sibling of a panicking method ran to its end or its AfterEach ran:\n%s", markers)
	}
}

// A failed assertion says what it could not work with: the type ErrorAs
// expected, the needle a haystack cannot hold, and every extra argument.
func (s *CanaryTestSuite) TestAssertionFailuresSayWhatTheyCannotWorkWith(t *gotest.T) {
	s.check(t, "failing")
	for _, want := range []string{"is not a *fs.PathError", "a []int cannot hold a string", "42 retries left", "delta is NaN"} {
		if !strings.Contains(s.lastStream, want) {
			t.Errorf("failing: no failure message says %q", want)
		}
	}
}

// A snapshot section is the exact name testing gave the subtest, found
// beside the test under -trimpath too; an update switch spelled off is off.
func (s *CanaryTestSuite) TestSnapshotsKeyOnTheExactName(t *gotest.T) {
	want := s.expected["snapshotting"]
	baseline := filepath.Join("testdata", "snapshotting", "testdata", "__snapshots__", "TestSnapshottingTestSuite_ext.snap")
	before, err := os.ReadFile(baseline)
	if err != nil {
		fatalf(t, "baseline: %v", err)
	}
	for _, c := range []struct {
		how   string
		env   []string
		flags []string
	}{
		{"built with -trimpath", nil, []string{"-trimpath"}},
		{"with GOTEST_UPDATE_SNAPSHOTS=0", []string{"GOTEST_UPDATE_SNAPSHOTS=0"}, nil},
		{"with GOTEST_UPDATE_SNAPSHOTS=false", []string{"GOTEST_UPDATE_SNAPSHOTS=false"}, nil},
	} {
		code, rows := s.runEnv(t, "snapshotting", t.TempDir(), c.env, c.flags...)
		if code != want.exit || strings.Join(rows, "\n") != strings.Join(want.rows, "\n") {
			t.Errorf("snapshotting %s: verdicts differ from the golden list (exit %d)\n got:\n  %s", c.how, code, strings.Join(rows, "\n  "))
		}
	}
	after, _ := os.ReadFile(baseline)
	if string(after) != string(before) {
		t.Errorf("snapshotting: a run with the update switch off rewrote the baseline")
		_ = os.WriteFile(baseline, before, 0o600)
	}
}

// An async method whose done-owning goroutine fails fatally fails at once:
// nothing is left to call done, so nothing waits for the budget.
func (s *CanaryTestSuite) TestAFatalGoroutineEndsAnAsyncWait(t *gotest.T) {
	began := time.Now()
	code, rows := s.run(t, "asyncfatal", t.TempDir())
	if code != 1 {
		t.Errorf("asyncfatal: exit code %d, want 1", code)
	}
	if !slices.Contains(rows, "asyncfatal fail TestAsyncTestSuite/TestFatalGoroutineAsync") {
		t.Errorf("asyncfatal: the method did not fail: %v", rows)
	}
	if took := time.Since(began); took > 10*time.Second {
		t.Errorf("asyncfatal: the run took %s; the wait outlived the failed goroutine", took)
	}
}

// A run stopped while a method holds: the methods not yet started are skipped
// with the reason run stopping, the holding method's context ends, and AfterAll
// runs before the fixture is released. Ring 0: the conformance suite that
// states the same cannot judge its own assertion engine.
func (s *CanaryTestSuite) TestAStopSkipsTheMethodsNotStarted(t *gotest.T) {
	dir := t.TempDir()
	cmd := exec.Command(s.binary, "-json", "./testdata/stopping/") //nolint:gosec // G204: controlled binary with fixed args
	cmd.Env = childEnv(dir)
	var stdout, stderr bytes.Buffer
	cmd.Stdout, cmd.Stderr = &stdout, &stderr
	cmd.WaitDelay = 15 * time.Second
	tree, err := testkit.StartCLI(cmd)
	if err != nil {
		fatalf(t, "start: %v", err)
	}
	markers := filepath.Join(dir, testkit.MarkersFile)
	for deadline := time.Now().Add(90 * time.Second); ; time.Sleep(20 * time.Millisecond) {
		got, _ := testkit.ReadMarkers(markers)
		if slices.Contains(got, "running") {
			break
		}
		if time.Now().After(deadline) {
			_ = tree.Kill()
			_ = cmd.Wait()
			tree.Release()
			fatalf(t, "stop: the run never held:\n%s%s", stdout.String(), stderr.String())
		}
	}
	if err := testkit.Interrupt(cmd, tree); err != nil {
		_ = tree.Kill()
		_ = cmd.Wait()
		tree.Release()
		fatalf(t, "stop: signal: %v", err)
	}
	_ = cmd.Wait()
	tree.Release()
	if code := cmd.ProcessState.ExitCode(); code != 130 {
		t.Errorf("stop: exit code %d, want 130\n%s", code, stderr.String())
	}
	verdict := map[string]string{}
	output := map[string]string{}
	for _, ev := range testkit.ParseEvents(stdout.Bytes()) {
		switch ev.Action {
		case "pass", "fail", "skip":
			verdict[ev.Test] = ev.Action
		case "output":
			output[ev.Test] += ev.Output
		}
	}
	for _, method := range []string{"TestStopTestSuite/TestSecond", "TestStopTestSuite/TestThird"} {
		if verdict[method] != "skip" {
			t.Errorf("stop: %s verdict %q, want skip", method, verdict[method])
		}
		if !strings.Contains(output[method], "run stopping") {
			t.Errorf("stop: %s was not skipped with the reason run stopping:\n%s", method, output[method])
		}
	}
	got, _ := testkit.ReadMarkers(markers)
	order := []string{"context ended", "suite.AfterAll", "fixture.AfterAll"}
	last := -1
	for _, m := range order {
		idx := slices.Index(got, m)
		if idx < 0 || idx < last {
			t.Errorf("stop: markers %v, want %v in order", got, order)
			break
		}
		last = idx
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
func (s *CanaryTestSuite) bench(t *gotest.T, pkg, canaryDir string) (int, []string) {
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
func (s *CanaryTestSuite) benchReport(t *gotest.T, pkg, canaryDir string, extra ...string) (int, benchReport) {
	code, stdout, stderr := s.benchRaw(t, pkg, canaryDir, extra...)
	var report benchReport
	if err := json.Unmarshal(stdout, &report); err != nil {
		fatalf(t, "bench %s: report is not the JSON document bench --json promises: %v\nstdout:\n%s\nstderr:\n%s", pkg, err, stdout, stderr)
	}
	return code, report
}

// benchRaw runs bench --json over one fixture package and returns the exit
// code and both streams as they were.
func (s *CanaryTestSuite) benchRaw(t *gotest.T, pkg, canaryDir string, extra ...string) (int, []byte, []byte) {
	args := append([]string{"bench", "--json"}, extra...)
	args = append(args, "./testdata/"+pkg+"/", "-benchtime=1x")
	return s.invoke(t, canaryDir, args...)
}

// invoke runs the CLI with the arguments as given and returns the exit code
// and both streams as they were.
func (s *CanaryTestSuite) invoke(t *gotest.T, canaryDir string, args ...string) (int, []byte, []byte) {
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
