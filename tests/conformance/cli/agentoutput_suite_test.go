package cli_test

import (
	"encoding/json"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"github.com/mvrahden/go-test/internal/testkit"
	"github.com/mvrahden/go-test/pkg/gotest"
)

// CLIAgentOutputTestSuite: a text run an agent cannot misread: what failed
// and a closing line, a census, and a rerun of what failed.
type CLIAgentOutputTestSuite struct{}

func (s *CLIAgentOutputTestSuite) SuiteConfig() gotest.SuiteConfig {
	return gotest.SuiteConfig{Parallel: true, Timeout: time.Minute} // up to four CLI runs in one method
}

// lastLine is the last line of out.
func lastLine(out string) string {
	lines := strings.Split(strings.TrimRight(out, "\n"), "\n")
	return lines[len(lines)-1]
}

// inCache is the environment of a run whose gotest cache is dir, the agent
// variables after it.
func inCache(dir string, env ...string) []string {
	return append([]string{"GOTEST_CACHE_DIR=" + dir}, env...)
}

func (s *CLIAgentOutputTestSuite) TestAFailingRunUnderAnAgent(t *gotest.T) {
	run, err := testkit.RunCLIWithEnv(t.Context(), ".", inCache(t.TempDir(), "CLAUDECODE=1"), "../testdata/failing/", "../testdata/passing/")
	gotest.NoError(t, err)
	gotest.Equal(t, 1, run.Exit, "stdout: %s\nstderr: %s", run.Stdout, run.Stderr)

	t.When("a text run of a failing and a passing package is made under CLAUDECODE=1", func(w *gotest.T) {
		w.It("prints on stdout the failing method's output and no line of the passing package", func(it *gotest.T) {
			gotest.Contains(it, run.Stdout, "failing_suite_test.go:9: Fail\n")
			gotest.Contains(it, run.Stdout, "--- FAIL: TestFailingTestSuite/TestFails (")
			gotest.NotContains(it, run.Stdout, "testdata/passing")
		})
		w.It("ends stdout with the closing line naming FAIL, exit 1, the tests counted and the command that reruns the failures", func(it *gotest.T) {
			gotest.Equal(it, "gotest: FAIL exit 1 | tests: 2 passed, 2 failed, 0 skipped | packages: 1 passed, 1 failed, 0 skipped | rerun: go run github.com/mvrahden/go-test/cmd/gotest --failed", lastLine(run.Stdout))
		})
		w.It("says first on stderr that agent output is on under CLAUDECODE=1 and that GOTEST_AGENT=0 turns it off", func(it *gotest.T) {
			first, _, _ := strings.Cut(run.Stderr, "\n")
			gotest.Regexp(it, `^gotest: agent output on \(CLAUDECODE=1\): .*GOTEST_AGENT=0 turns it off$`, first)
		})
	})
}

func (s *CLIAgentOutputTestSuite) TestAgentOutputTurnedOff(t *gotest.T) {
	t.When("a text run under CLAUDECODE=1 is given --agent=false, or GOTEST_AGENT=0", func(w *gotest.T) {
		w.It("prints go test's text, the passing package's ok line included, and no word of agent output", func(it *gotest.T) {
			for sub, r := range gotest.Each(it, []struct {
				Desc string
				env  []string
				args []string
			}{
				{Desc: "--agent=false", env: []string{"CLAUDECODE=1"}, args: []string{"--agent=false", "../testdata/passing/"}},
				{Desc: "GOTEST_AGENT=0", env: []string{"CLAUDECODE=1", "GOTEST_AGENT=0"}, args: []string{"../testdata/passing/"}},
			}) {
				run, err := testkit.RunCLIWithEnv(sub.Context(), ".", inCache(sub.TempDir(), r.env...), r.args...)
				gotest.NoError(sub, err)
				gotest.Equal(sub, 0, run.Exit, "stderr: %s", run.Stderr)
				gotest.Regexp(sub, `^ok  \t\S+/testdata/passing\t\d+\.\d{3}s\n$`, run.Stdout)
				gotest.NotContains(sub, run.Stderr, "agent output")
			}
		})
	})
}

func (s *CLIAgentOutputTestSuite) TestAgentOutputAskedFor(t *gotest.T) {
	t.When("a text run with no agent variable is given --agent, or GOTEST_AGENT=1", func(w *gotest.T) {
		w.It("prints no ok line and ends stdout with the closing line", func(it *gotest.T) {
			for sub, r := range gotest.Each(it, []struct {
				Desc string
				env  []string
				args []string
			}{
				{Desc: "--agent", args: []string{"--agent", "../testdata/passing/"}},
				{Desc: "GOTEST_AGENT=1", env: []string{"GOTEST_AGENT=1"}, args: []string{"../testdata/passing/"}},
			}) {
				run, err := testkit.RunCLIWithEnv(sub.Context(), ".", inCache(sub.TempDir(), r.env...), r.args...)
				gotest.NoError(sub, err)
				gotest.Equal(sub, 0, run.Exit, "stderr: %s", run.Stderr)
				gotest.Equal(sub, "gotest: PASS exit 0 | tests: 2 passed, 0 failed, 0 skipped, 0 not run | packages: 1 passed, 0 failed, 0 skipped\n", run.Stdout)
			}
		})
	})
}

func (s *CLIAgentOutputTestSuite) TestAPassingTextRun(t *gotest.T) {
	run, err := testkit.RunCLIWithEnv(t.Context(), ".", inCache(t.TempDir()), "../testdata/passing/")
	gotest.NoError(t, err)
	gotest.Equal(t, 0, run.Exit, "stderr: %s", run.Stderr)

	t.When("a text run of a passing package ends", func(w *gotest.T) {
		w.It("prints on stdout go test's text alone", func(it *gotest.T) {
			gotest.Regexp(it, `^ok  \t\S+/testdata/passing\t\d+\.\d{3}s\n$`, run.Stdout)
		})
		w.It("ends stderr with the closing line naming PASS, exit 0 and 1 package passed", func(it *gotest.T) {
			gotest.Equal(it, "gotest: PASS exit 0 | packages: 1 passed, 0 failed, 0 skipped", lastLine(run.Stderr))
		})
	})
}

func (s *CLIAgentOutputTestSuite) TestARunGivenCountZero(t *gotest.T) {
	t.When("a text or a -json run is given -count=0", func(w *gotest.T) {
		w.It("passes, noting that the census was skipped", func(it *gotest.T) {
			for sub, r := range gotest.Each(it, []struct {
				Desc string
				args []string
			}{
				{Desc: "a text run", args: []string{"-count=0", "../testdata/passing/"}},
				{Desc: "a -json run", args: []string{"-json", "-count=0", "../testdata/passing/"}},
			}) {
				run, err := testkit.RunCLIWithEnv(sub.Context(), ".", inCache(sub.TempDir()), r.args...)
				gotest.NoError(sub, err)
				gotest.Equal(sub, 0, run.Exit, "stderr: %s", run.Stderr)
				gotest.Contains(sub, run.Stderr, "note: census skipped under -count=0\n")
			}
		})
	})
}

// runRecord is a run's record, as --failed reads it.
type runRecord struct {
	Result struct{ ExitCode int }
	Failed []struct{ Package, Test string }
}

// files lists the files under dir.
func files(t *gotest.T, dir string) []string {
	var out []string
	gotest.NoError(t, filepath.WalkDir(dir, func(path string, d fs.DirEntry, err error) error {
		if err == nil && !d.IsDir() {
			out = append(out, path)
		}
		return err
	}))
	return out
}

func (s *CLIAgentOutputTestSuite) TestTheRecordOfAFailingRun(t *gotest.T) {
	root, err := testkit.RepoRoot()
	gotest.NoError(t, err)
	module, cache := t.TempDir(), t.TempDir()
	_, err = testkit.StagePackage(root, filepath.Join("..", "testdata", "failing"), module, "recordmod")
	gotest.NoError(t, err)
	before := files(t, module)
	run, err := testkit.RunCLIWithEnv(t.Context(), module, inCache(cache), "./failing/")
	gotest.NoError(t, err)
	gotest.Equal(t, 1, run.Exit, "stderr: %s", run.Stderr)
	records, err := filepath.Glob(filepath.Join(cache, "runs", "*", "last.json"))
	gotest.NoError(t, err)

	t.When("a text run of a failing package ends", func(w *gotest.T) {
		w.It("its record names the failing method and exit code 1", func(it *gotest.T) {
			gotest.Len(it, records, 1)
			data, err := os.ReadFile(records[0])
			gotest.NoError(it, err)
			var rec runRecord
			gotest.NoError(it, json.Unmarshal(data, &rec))
			gotest.Equal(it, 1, rec.Result.ExitCode)
			gotest.Equal(it, []struct{ Package, Test string }{{"recordmod/failing", "TestFailingTestSuite/TestFails"}}, rec.Failed)
		})
		w.It("its record holds no test counts", func(it *gotest.T) {
			gotest.Len(it, records, 1)
			data, err := os.ReadFile(records[0])
			gotest.NoError(it, err)
			var rec struct{ Result map[string]json.RawMessage }
			gotest.NoError(it, json.Unmarshal(data, &rec))
			gotest.Contains(it, rec.Result, "Packages")
			gotest.NotContains(it, rec.Result, "Tests")
		})
		w.It("the module holds no file it did not hold before", func(it *gotest.T) {
			gotest.Equal(it, before, files(it, module))
		})
	})
}

// testsRun are the tests the stream started, in order.
func testsRun(run *testkit.Run) []string {
	var out []string
	for _, e := range run.Events {
		if e.Action == "run" && !slices.Contains(out, e.Test) {
			out = append(out, e.Test)
		}
	}
	return out
}

func (s *CLIAgentOutputTestSuite) TestRerunningAFailedMethod(t *gotest.T) {
	env := inCache(t.TempDir())
	first, err := testkit.RunCLIWithEnv(t.Context(), ".", env, "../testdata/clirerun/", "../testdata/passing/")
	gotest.NoError(t, err)
	gotest.Equal(t, 1, first.Exit, "stderr: %s", first.Stderr)
	text, err := testkit.RunCLIWithEnv(t.Context(), ".", env, "--failed", "-v")
	gotest.NoError(t, err)
	stream, err := testkit.RunCLIWithEnv(t.Context(), ".", env, "-json", "--failed")
	gotest.NoError(t, err)
	spec, err := testkit.RunCLIWithEnv(t.Context(), ".", env, "spec", "--failed", "--no-color")
	gotest.NoError(t, err)

	t.When("--failed follows a run in which one method of a suite failed and one passed, beside a passing package", func(w *gotest.T) {
		w.It("runs only the failed method and exits 1", func(it *gotest.T) {
			gotest.Equal(it, 1, text.Exit, "stderr: %s", text.Stderr)
			gotest.Contains(it, text.Stdout, "=== RUN   TestRerunTestSuite/TestFails\n")
			gotest.NotContains(it, text.Stdout, "TestPasses")
			gotest.NotContains(it, text.Stdout, "testdata/passing")
		})
		w.It("-json --failed streams the failed method's verdict and no other method's", func(it *gotest.T) {
			gotest.Equal(it, 1, stream.Exit, "stderr: %s", stream.Stderr)
			gotest.Equal(it, []string{"TestRerunTestSuite", "TestRerunTestSuite/TestFails"}, testsRun(&stream))
		})
		w.It("spec --failed renders the failed method and no other", func(it *gotest.T) {
			gotest.Equal(it, 1, spec.Exit, "stderr: %s", spec.Stderr)
			gotest.Contains(it, spec.Stdout, "✗ Fails")
			gotest.NotContains(it, spec.Stdout, "Passes")
			gotest.Contains(it, spec.Stdout, "1 suite, 1 behavior: 1 failed")
		})
	})
}

func (s *CLIAgentOutputTestSuite) TestNothingToRerun(t *gotest.T) {
	t.When("--failed is given without a record, after a run that failed nothing, or with a package that did not fail", func(w *gotest.T) {
		w.It("runs nothing and exits 2, saying why on stderr", func(it *gotest.T) {
			for sub, r := range gotest.Each(it, []struct {
				Desc   string
				before []string
				args   []string
				says   string
			}{
				{Desc: "no record", args: []string{"--failed"}, says: "FAIL: --failed: no run is recorded for "},
				{Desc: "a run that failed nothing", before: []string{"../testdata/passing/"}, args: []string{"--failed"}, says: "exited 0 and failed no test, so there is nothing to rerun"},
				{Desc: "a package that did not fail", before: []string{"../testdata/failing/", "../testdata/passing/"}, args: []string{"--failed", "../testdata/passing/"}, says: "FAIL: --failed: none of the packages given failed in the last run"},
			}) {
				env := inCache(sub.TempDir())
				if r.before != nil {
					_, err := testkit.RunCLIWithEnv(sub.Context(), ".", env, r.before...)
					gotest.NoError(sub, err)
				}
				run, err := testkit.RunCLIWithEnv(sub.Context(), ".", env, r.args...)
				gotest.NoError(sub, err)
				gotest.Equal(sub, 2, run.Exit, "stdout: %s\nstderr: %s", run.Stdout, run.Stderr)
				gotest.Empty(sub, run.Stdout)
				gotest.Contains(sub, run.Stderr, r.says)
			}
		})
	})
}
