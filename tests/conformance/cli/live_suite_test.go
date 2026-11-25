package cli_test

import (
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"github.com/mvrahden/go-test/internal/proctree"
	"github.com/mvrahden/go-test/internal/protocol"
	"github.com/mvrahden/go-test/internal/testkit"
	"github.com/mvrahden/go-test/pkg/gotest"
)

// CLILiveTestSuite: a --live -json run writes each suite's events as the
// suite process writes them. Each suite of clilive holds in its second method
// until the test releases it, so what the stream holds before the release
// was written while both suites ran.
// Sequential: it builds the CLI once and every row drives a live process.
type CLILiveTestSuite struct {
	cli string
	// cache holds the test binaries, built before any row so that no row
	// compiles inside the window it waits in.
	cache string
}

func (s *CLILiveTestSuite) SuiteConfig() gotest.SuiteConfig {
	// The setup builds the CLI and, cold, the test binary; the --timeout row
	// waits for the deadline, then for a method that ignores it to be killed.
	return gotest.SuiteConfig{Timeout: 60 * time.Second, SetupTimeout: 90 * time.Second}
}

func (s *CLILiveTestSuite) BeforeAll(t *gotest.T) {
	root, err := testkit.RepoRoot()
	gotest.NoError(t, err)
	s.cli, err = testkit.BuildCLI(t.Context(), root, t.TempDir())
	gotest.NoError(t, err)
	testkit.ScrubActionsEnv()
	s.cache = testkit.ChildCacheDir(t.TempDir())
	// Without LIVE_RELEASE_DIR nothing holds: a real run that builds.
	cmd := exec.CommandContext(t.Context(), s.cli, "-json", livePkg) //nolint:gosec // G204: binary built by this suite
	cmd.Env = testkit.ChildEnv(t.TempDir(), protocol.EnvCacheDir+"="+s.cache)
	out, err := cmd.CombinedOutput()
	gotest.NoError(t, err, "the warm-up run:\n%s", out)
}

// AfterAll has nothing to release: the binary and the cache live in temp dirs.
func (s *CLILiveTestSuite) AfterAll(t *gotest.T) {}

const (
	livePkg     = "../testdata/clilive/"
	firstSuite  = "TestFirstTestSuite"
	secondSuite = "TestSecondTestSuite"
	// liveWithin is how long an event may take to reach stdout once its
	// suite wrote it; BeforeAll built the binary.
	liveWithin = 20 * time.Second
)

// liveRun is a run of clilive whose stdout and events file a test reads
// while it runs.
type liveRun struct {
	cmd                   *exec.Cmd
	tree                  *proctree.Tree
	released, out, events string
}

// start runs clilive with args and env added; its suites hold until
// release lets them go.
func (s *CLILiveTestSuite) start(t *gotest.T, args []string, env ...string) *liveRun {
	dir := t.TempDir()
	r := &liveRun{released: t.TempDir(), out: filepath.Join(dir, "stdout"), events: filepath.Join(dir, "events.json")}
	out, err := os.Create(r.out)
	gotest.NoError(t, err)
	defer out.Close()
	args = append(append([]string{"--events-file=" + r.events}, args...), livePkg)
	r.cmd = exec.Command(s.cli, args...) //nolint:gosec // G204: binary built by this suite
	r.cmd.Env = testkit.ChildEnv(t.TempDir(), append([]string{protocol.EnvCacheDir + "=" + s.cache, "LIVE_RELEASE_DIR=" + r.released}, env...)...)
	r.cmd.Stdout, r.cmd.Stderr = out, new(strings.Builder)
	r.cmd.WaitDelay = 15 * time.Second
	r.tree, err = testkit.StartCLI(r.cmd)
	gotest.NoError(t, err)
	return r
}

// await returns stdout once holds says it holds what was awaited, or once
// liveWithin has passed: the caller releases the run either way, and its
// clauses judge what stdout held.
func (r *liveRun) await(holds func(stdout string) bool) string {
	for deadline := time.Now().Add(testkit.Scaled(liveWithin)); ; time.Sleep(20 * time.Millisecond) {
		got, _ := os.ReadFile(r.out)
		if holds(string(got)) || time.Now().After(deadline) {
			return string(got)
		}
	}
}

// release lets the named suites' holding methods return.
func (r *liveRun) release(t *gotest.T, names ...string) {
	for _, name := range names {
		gotest.NoError(t, os.WriteFile(filepath.Join(r.released, name), nil, 0o600))
	}
}

// finish waits for the run and returns its stdout, its events file and its
// exit code.
func (r *liveRun) finish(t *gotest.T) (stdout, file string, code int) {
	err := r.cmd.Wait()
	r.tree.Release()
	if exit := (*exec.ExitError)(nil); errors.As(err, &exit) {
		code = exit.ExitCode()
	} else {
		gotest.NoError(t, err)
	}
	out, err := os.ReadFile(r.out)
	gotest.NoError(t, err)
	events, err := os.ReadFile(r.events)
	gotest.NoError(t, err)
	return string(out), string(events), code
}

// eventIndex is the position of the first event of stream that is action on
// test, -1 for none; test "" is the package's own.
func eventIndex(stream, action, test string) int {
	return slices.IndexFunc(testkit.ParseEvents([]byte(stream)), func(ev testkit.Event) bool {
		return ev.Action == action && ev.Test == test && strings.HasSuffix(ev.Package, "/testdata/clilive")
	})
}

func passed(stream, test string) bool { return eventIndex(stream, "pass", test) >= 0 }

// firstPasses says whether stream holds both suites' first passes.
func firstPasses(stream string) bool {
	return passed(stream, firstSuite+"/TestAPasses") && passed(stream, secondSuite+"/TestAPasses")
}

// suiteEvents are the action and test of each event of stream under suite,
// in stream order.
func suiteEvents(stream, suite string) []string {
	var out []string
	for _, ev := range testkit.ParseEvents([]byte(stream)) {
		if ev.Test == suite || strings.HasPrefix(ev.Test, suite+"/") {
			out = append(out, ev.Action+" "+ev.Test)
		}
	}
	return out
}

func (s *CLILiveTestSuite) TestTwoSuitesThatHold(t *gotest.T) {
	r := s.start(t, []string{"-json", "--live"})
	before := r.await(firstPasses)
	r.release(t, "first", "second")
	stdout, file, code := r.finish(t)

	// The same run without --live, its suites released before they start.
	base := s.start(t, []string{"-json"})
	base.release(t, "first", "second")
	baseline, _, baseCode := base.finish(t)

	t.When("a -json run is given --live while each of its two suites holds after its first method", func(w *gotest.T) {
		w.It("writes each suite's first pass on stdout before either suite ends", func(it *gotest.T) {
			gotest.True(it, firstPasses(before), "stdout before the release:\n%s", before)
			gotest.False(it, passed(before, firstSuite+"/TestBHolds"), "stdout before the release:\n%s", before)
			gotest.False(it, passed(before, secondSuite+"/TestBHolds"), "stdout before the release:\n%s", before)
		})
		w.It("writes the package's verdict only after both suites ended", func(it *gotest.T) {
			verdict := eventIndex(stdout, "pass", "")
			gotest.Greater(it, verdict, eventIndex(stdout, "pass", firstSuite), "stdout:\n%s", stdout)
			gotest.Greater(it, verdict, eventIndex(stdout, "pass", secondSuite), "stdout:\n%s", stdout)
		})
		w.It("writes each suite's events once each, in the order the suite wrote them, every line one whole event", func(it *gotest.T) {
			gotest.Equal(it, 0, code)
			gotest.Equal(it, 0, baseCode)
			gotest.Empty(it, incompleteLines(stdout), "stdout:\n%s", stdout)
			for _, suite := range []string{firstSuite, secondSuite} {
				gotest.NotEmpty(it, suiteEvents(baseline, suite))
				gotest.Equal(it, suiteEvents(baseline, suite), suiteEvents(stdout, suite), "the run without --live:\n%s", baseline)
			}
		})
		w.It("writes the events file byte for byte as stdout", func(it *gotest.T) {
			gotest.Equal(it, stdout, file)
		})
	})
}

func (s *CLILiveTestSuite) TestTheVariable(t *gotest.T) {
	r := s.start(t, []string{"-json"}, protocol.EnvLive+"=1")
	before := r.await(firstPasses)
	r.release(t, "first", "second")
	_, _, code := r.finish(t)

	t.When("a -json run is made under GOTEST_LIVE=1 while its suites hold", func(w *gotest.T) {
		w.It("writes each suite's first pass on stdout before either suite ends", func(it *gotest.T) {
			gotest.True(it, firstPasses(before), "stdout before the release:\n%s", before)
			gotest.False(it, passed(before, firstSuite+"/TestBHolds"), "stdout before the release:\n%s", before)
			gotest.Equal(it, 0, code)
		})
	})
}

func (s *CLILiveTestSuite) TestATextRun(t *gotest.T) {
	run, err := testkit.RunCLI(t.Context(), ".", "--live", livePkg)
	gotest.NoError(t, err)

	t.When("--live is given without -json", func(w *gotest.T) {
		w.It("refuses it with exit 2, naming -json", func(it *gotest.T) {
			gotest.Equal(it, 2, run.Exit, "stdout: %s\nstderr: %s", run.Stdout, run.Stderr)
			gotest.Contains(it, run.Stderr, "FAIL: --live")
			gotest.Contains(it, run.Stderr, "add -json")
		})
	})
}

func (s *CLILiveTestSuite) TestAMethodThatHangsPastTheTimeout(t *gotest.T) {
	// The second suite's method ignores its context: only the deadline's
	// kill ends it, and no stop line names it.
	r := s.start(t, []string{"-json", "--live", "--timeout=" + testkit.Scaled(8*time.Second).String()}, "LIVE_IGNORE_CONTEXT=1")
	held := r.await(func(stdout string) bool {
		return eventIndex(stdout, "run", firstSuite+"/TestBHolds") >= 0 && eventIndex(stdout, "run", secondSuite+"/TestBHolds") >= 0
	})
	r.release(t, "first")
	stdout, _, code := r.finish(t)

	t.When("--timeout expires in a --live run while one suite's method hangs after the other suite ended", func(w *gotest.T) {
		w.It("names the hanging method in the stream with the line that names the deadline", func(it *gotest.T) {
			gotest.Greater(it, eventIndex(held, "run", secondSuite+"/TestBHolds"), -1, "stdout before the release:\n%s", held)
			gotest.Equal(it, 1, code, "stdout:\n%s", stdout)
			gotest.True(it, passed(stdout, firstSuite), "stdout:\n%s", stdout)
			gotest.Contains(it, stdout, `"Test":"`+secondSuite+`/TestBHolds","Output":"    gotest: --timeout expired while this test was running\n"`)
			gotest.NotContains(it, stdout, `"Test":"`+firstSuite+`/TestBHolds","Output":"    gotest: --timeout expired`)
		})
	})
}
