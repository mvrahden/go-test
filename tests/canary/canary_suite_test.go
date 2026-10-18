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
func (s *CanaryTestSuite) run(t *gotest.T, pkg, canaryDir string) (int, []string) {
	cmd := exec.Command(s.binary, "-json", "./testdata/"+pkg+"/") //nolint:gosec // G204: controlled binary with fixed args
	cmd.Env = childEnv(canaryDir)
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
