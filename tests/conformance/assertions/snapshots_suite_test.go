package assertions_test

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/mvrahden/go-test/internal/testkit"
	"github.com/mvrahden/go-test/pkg/gotest"
)

// SnapshotKeysTestSuite: a snapshot section is the exact name testing gave
// the subtest, found beside the test whatever the build flags.
type SnapshotKeysTestSuite struct{}

func (s *SnapshotKeysTestSuite) SuiteConfig() gotest.SuiteConfig {
	return gotest.SuiteConfig{Parallel: true, Timeout: 45 * time.Second}
}

func (s *SnapshotKeysTestSuite) TestKeys(t *gotest.T) {
	run, err := testkit.RunPackageWithEnv(t.Context(), t.TempDir(), []string{"GOTEST_CI=1"}, "../testdata/snapkeys/")
	gotest.NoError(t, err)
	t.When("Each rows and same-named subtests match snapshots", func(w *gotest.T) {
		w.It("#0, #1, x and x#01 each read their own section", func(it *gotest.T) {
			for _, name := range []string{"TestRows/#0", "TestRows/#1", "TestTwins/x", "TestTwins/x#01"} {
				gotest.Equal(it, "pass", run.Verdict("TestSnapKeysTestSuite/"+name), "stdout: %s", run.Stdout)
			}
		})
	})
	t.When("MatchSnapshot is given the recorder of an Eventually or Consistently poll", func(w *gotest.T) {
		w.It("reads the section of the test that polls", func(it *gotest.T) {
			gotest.Equal(it, "pass", run.Verdict("TestSnapKeysTestSuite/TestInAPoll"), "stdout: %s", run.Stdout)
		})
	})
}

func (s *SnapshotKeysTestSuite) TestLineEnds(t *gotest.T) {
	run, err := testkit.RunPackageWithEnv(t.Context(), t.TempDir(), []string{"GOTEST_CI=1"}, "../testdata/snapkeys/")
	gotest.NoError(t, err)
	t.When("a value ends lines with runs of carriage returns, or ends in one", func(w *gotest.T) {
		w.It("matches the baseline written with plain newlines", func(it *gotest.T) {
			gotest.Equal(it, "pass", run.Verdict("TestSnapKeysTestSuite/TestCarriageReturns"), "stdout: %s", run.Stdout)
		})
	})
}

func (s *SnapshotKeysTestSuite) TestTrimpath(t *gotest.T) {
	run, err := testkit.RunPackageWithEnv(t.Context(), t.TempDir(), []string{"GOTEST_CI=1"}, "../testdata/snapkeys/", "-trimpath")
	gotest.NoError(t, err)
	t.When("the package is built with -trimpath", func(w *gotest.T) {
		w.It("the baselines are still found beside the test, _ext suffix included", func(it *gotest.T) {
			gotest.Equal(it, "pass", run.Verdict("TestSnapKeysTestSuite/TestTwins/x#01"), "stdout: %s", run.Stdout)
			gotest.Equal(it, "pass", run.Verdict("TestSnapKeysTestSuite/TestRows/#1"))
		})
	})
}

func (s *SnapshotKeysTestSuite) TestUpdateSwitchOff(t *gotest.T) {
	// A staged copy: a switch that is not off rewrites this baseline, never
	// the committed one the other methods read.
	root, err := testkit.RepoRoot()
	gotest.NoError(t, err)
	staged := t.TempDir()
	pkg, err := testkit.StagePackage(root, filepath.Join("..", "testdata", "snapkeys"), staged, "snapmod")
	gotest.NoError(t, err)
	baseline := filepath.Join(pkg, "testdata", "__snapshots__", "TestSnapKeysTestSuite_ext.snap")
	before, err := os.ReadFile(baseline) //nolint:gosec // G304: the staged copy of the repo's own fixture
	gotest.NoError(t, err)
	run, err := testkit.RunCLIWithEnv(t.Context(), staged, []string{"GOTEST_CI=1", "GOTEST_UPDATE_SNAPSHOTS=0"}, "-json", "./snapkeys/")
	gotest.NoError(t, err)
	after, err := os.ReadFile(baseline) //nolint:gosec // G304: the staged copy of the repo's own fixture
	gotest.NoError(t, err)
	t.When("GOTEST_UPDATE_SNAPSHOTS is 0", func(w *gotest.T) {
		w.It("a stale baseline still fails and is left as it was", func(it *gotest.T) {
			gotest.Equal(it, "fail", run.Verdict("TestSnapKeysTestSuite/TestStale"), "stdout: %s", run.Stdout)
			gotest.Equal(it, string(before), string(after), "the baseline was rewritten")
		})
	})
}

func (s *SnapshotKeysTestSuite) TestSwitchSpellings(t *gotest.T) {
	root, err := testkit.RepoRoot()
	gotest.NoError(t, err)
	staged := t.TempDir()
	pkg, err := testkit.StagePackage(root, filepath.Join("..", "testdata", "snapkeys"), staged, "snapspellmod")
	gotest.NoError(t, err)
	baseline := filepath.Join(pkg, "testdata", "__snapshots__", "TestSnapKeysTestSuite_ext.snap")
	without := strings.Replace(readFile(t, baseline), "=== SNAP TestRows/#1 ===\nrow one\n", "", 1)
	gotest.NoError(t, os.WriteFile(baseline, []byte(without), 0o600))
	run, err := testkit.RunCLIWithEnv(t.Context(), staged, []string{"GOTEST_CI=FALSE", "GOTEST_UPDATE_SNAPSHOTS=f"}, "-json", "./snapkeys/")
	gotest.NoError(t, err)
	after := readFile(t, baseline)
	t.When("GOTEST_CI and GOTEST_UPDATE_SNAPSHOTS are spelled FALSE and f", func(w *gotest.T) {
		w.It("both are off: a missing baseline is written and a stale one still fails", func(it *gotest.T) {
			gotest.Equal(it, "pass", run.Verdict("TestSnapKeysTestSuite/TestRows/#1"), "stdout: %s", run.Stdout)
			gotest.Contains(it, after, "=== SNAP TestRows/#1 ===\nrow one\n")
			gotest.Equal(it, "fail", run.Verdict("TestSnapKeysTestSuite/TestStale"))
			gotest.Contains(it, after, "=== SNAP TestStale ===\nold\n")
		})
	})
}

// switchHint ends the refusal of a switch value that is no boolean.
const switchHint = " is neither on nor off: set it to 1 or true to switch it on, to 0 or false to switch it off, or unset it"

// stageSnapKeys stages a copy of snapkeys as module, returning the module
// directory, the baseline's path and its content.
func stageSnapKeys(t *gotest.T, module string) (staged, baseline, before string) {
	root, err := testkit.RepoRoot()
	gotest.NoError(t, err)
	staged = t.TempDir()
	pkg, err := testkit.StagePackage(root, filepath.Join("..", "testdata", "snapkeys"), staged, module)
	gotest.NoError(t, err)
	baseline = filepath.Join(pkg, "testdata", "__snapshots__", "TestSnapKeysTestSuite_ext.snap")
	return staged, baseline, readFile(t, baseline)
}

func (s *SnapshotKeysTestSuite) TestValuesThatAreNoSwitch(t *gotest.T) {
	staged, baseline, before := stageSnapKeys(t, "snapnoswitchmod")
	stagedPlain, baselinePlain, beforePlain := stageSnapKeys(t, "snapplainmod")
	t.When("GOTEST_UPDATE_SNAPSHOTS or GOTEST_CI is set to a value that is no boolean", func(w *gotest.T) {
		w.It("the run ends with exit 2 before any test runs, naming the variable and the value", func(it *gotest.T) {
			for it, c := range gotest.Each(it, []struct {
				Desc  string
				env   []string
				args  []string
				names string
			}{
				{Desc: "GOTEST_UPDATE_SNAPSHOTS=off", env: []string{"GOTEST_UPDATE_SNAPSHOTS=off"}, args: []string{"-json", "./snapkeys/"}, names: `GOTEST_UPDATE_SNAPSHOTS="off"`},
				{Desc: "GOTEST_UPDATE_SNAPSHOTS=no", env: []string{"GOTEST_UPDATE_SNAPSHOTS=no"}, args: []string{"-json", "./snapkeys/"}, names: `GOTEST_UPDATE_SNAPSHOTS="no"`},
				{Desc: "GOTEST_UPDATE_SNAPSHOTS=2", env: []string{"GOTEST_UPDATE_SNAPSHOTS=2"}, args: []string{"-json", "./snapkeys/"}, names: `GOTEST_UPDATE_SNAPSHOTS="2"`},
				{Desc: "GOTEST_UPDATE_SNAPSHOTS=yes", env: []string{"GOTEST_UPDATE_SNAPSHOTS=yes"}, args: []string{"-json", "./snapkeys/"}, names: `GOTEST_UPDATE_SNAPSHOTS="yes"`},
				{Desc: "GOTEST_UPDATE_SNAPSHOTS is a space", env: []string{"GOTEST_UPDATE_SNAPSHOTS= "}, args: []string{"-json", "./snapkeys/"}, names: `GOTEST_UPDATE_SNAPSHOTS=" "`},
				{Desc: "GOTEST_UPDATE_SNAPSHOTS is 1 after a space", env: []string{"GOTEST_UPDATE_SNAPSHOTS= 1"}, args: []string{"-json", "./snapkeys/"}, names: `GOTEST_UPDATE_SNAPSHOTS=" 1"`},
				{Desc: "GOTEST_CI=off", env: []string{"GOTEST_CI=off"}, args: []string{"-json", "./snapkeys/"}, names: `GOTEST_CI="off"`},
				{Desc: "GOTEST_CI=yes", env: []string{"GOTEST_CI=yes"}, args: []string{"-json", "./snapkeys/"}, names: `GOTEST_CI="yes"`},
				{Desc: "both, GOTEST_CI named first", env: []string{"GOTEST_UPDATE_SNAPSHOTS=off", "GOTEST_CI=on"}, args: []string{"-json", "./snapkeys/"}, names: `GOTEST_CI="on"`},
				{Desc: "GOTEST_UPDATE_SNAPSHOTS=off beside --ci", env: []string{"GOTEST_UPDATE_SNAPSHOTS=off"}, args: []string{"--ci", "-json", "./snapkeys/"}, names: `GOTEST_UPDATE_SNAPSHOTS="off"`},
				{Desc: "GOTEST_CI=off beside --update-snapshots", env: []string{"GOTEST_CI=off"}, args: []string{"--update-snapshots", "-json", "./snapkeys/"}, names: `GOTEST_CI="off"`},
				{Desc: "GOTEST_UPDATE_SNAPSHOTS=off under summary", env: []string{"GOTEST_UPDATE_SNAPSHOTS=off"}, args: []string{"summary", "./snapkeys/"}, names: `GOTEST_UPDATE_SNAPSHOTS="off"`},
				{Desc: "GOTEST_UPDATE_SNAPSHOTS=off under spec", env: []string{"GOTEST_UPDATE_SNAPSHOTS=off"}, args: []string{"spec", "./snapkeys/"}, names: `GOTEST_UPDATE_SNAPSHOTS="off"`},
			}) {
				run, err := testkit.RunCLIWithEnv(it.Context(), staged, c.env, c.args...)
				gotest.NoError(it, err)
				gotest.Equal(it, 2, run.Exit, "stdout: %s\nstderr: %s", run.Stdout, run.Stderr)
				gotest.Contains(it, run.Stderr, "FAIL: "+c.names+switchHint+"\n")
				gotest.Empty(it, run.Stdout)
				gotest.Equal(it, before, readFile(it, baseline), "the baseline was rewritten")
			}
		})
		w.It("MatchSnapshot under plain go test fails naming the variable and the value, and leaves the baseline as it was", func(it *gotest.T) {
			gen, err := testkit.RunCLI(it.Context(), stagedPlain, "generate", "./snapkeys/")
			gotest.NoError(it, err)
			gotest.Equal(it, 0, gen.Exit, "stderr: %s", gen.Stderr)
			var stdout bytes.Buffer
			cmd := exec.CommandContext(it.Context(), "go", "test", "-json", "-count=1", "./snapkeys/")
			cmd.Dir = stagedPlain
			cmd.Env = testkit.ChildEnv(it.TempDir(), "GOTEST_UPDATE_SNAPSHOTS=off")
			cmd.Stdout = &stdout
			_ = cmd.Run() // the failing tests exit 1; the events say which
			verdicts := map[string]string{}
			var output strings.Builder
			for _, ev := range testkit.ParseEvents(stdout.Bytes()) {
				output.WriteString(ev.Output)
				switch ev.Action {
				case "pass", "fail", "skip":
					verdicts[ev.Test] = ev.Action
				}
			}
			gotest.Equal(it, "fail", verdicts["TestSnapKeysTestSuite/TestStale"], "go test output:\n%s", output.String())
			gotest.Empty(it, verdictsOf(verdicts, "pass"), "go test output:\n%s", output.String())
			gotest.Contains(it, output.String(), `snapkeys_suite_test.go:26: MatchSnapshot: GOTEST_UPDATE_SNAPSHOTS="off"`+switchHint)
			gotest.Equal(it, beforePlain, readFile(it, baselinePlain), "the baseline was rewritten")
		})
	})
}

// verdictsOf lists, sorted, the subtests whose verdict is want.
func verdictsOf(verdicts map[string]string, want string) []string {
	var names []string
	for name, v := range verdicts {
		if v == want && strings.Contains(name, "/") {
			names = append(names, name)
		}
	}
	sort.Strings(names)
	return names
}

func (s *SnapshotKeysTestSuite) TestTrueSpellings(t *gotest.T) {
	staged, baseline, before := stageSnapKeys(t, "snaptruemod")
	type spelled struct {
		value string
		run   testkit.Run
		after string
	}
	var runs []spelled
	for _, v := range []string{"true", "T"} {
		gotest.NoError(t, os.WriteFile(baseline, []byte(before), 0o600))
		run, err := testkit.RunCLIWithEnv(t.Context(), staged, []string{"GOTEST_UPDATE_SNAPSHOTS=" + v}, "-json", "./snapkeys/")
		gotest.NoError(t, err)
		runs = append(runs, spelled{value: v, run: run, after: readFile(t, baseline)})
	}
	t.When("GOTEST_UPDATE_SNAPSHOTS is spelled true or T", func(w *gotest.T) {
		w.It("a stale baseline passes and is rewritten", func(it *gotest.T) {
			for _, r := range runs {
				gotest.Equal(it, 0, r.run.Exit, "%s: stdout: %s\nstderr: %s", r.value, r.run.Stdout, r.run.Stderr)
				gotest.Equal(it, "pass", r.run.Verdict("TestSnapKeysTestSuite/TestStale"), r.value)
				gotest.Contains(it, r.after, "=== SNAP TestStale ===\nnew\n", r.value)
			}
		})
	})
}

func (s *SnapshotKeysTestSuite) TestEmptyValues(t *gotest.T) {
	staged, baseline, _ := stageSnapKeys(t, "snapemptymod")
	without := strings.Replace(readFile(t, baseline), "=== SNAP TestRows/#1 ===\nrow one\n", "", 1)
	gotest.NoError(t, os.WriteFile(baseline, []byte(without), 0o600))
	run, err := testkit.RunCLIWithEnv(t.Context(), staged, []string{"GOTEST_CI=", "GOTEST_UPDATE_SNAPSHOTS="}, "-json", "./snapkeys/")
	gotest.NoError(t, err)
	after := readFile(t, baseline)
	t.When("GOTEST_CI and GOTEST_UPDATE_SNAPSHOTS are set to the empty string", func(w *gotest.T) {
		w.It("both are off: a missing baseline is written and a stale one still fails", func(it *gotest.T) {
			gotest.Equal(it, "pass", run.Verdict("TestSnapKeysTestSuite/TestRows/#1"), "stdout: %s", run.Stdout)
			gotest.Contains(it, after, "=== SNAP TestRows/#1 ===\nrow one\n")
			gotest.Equal(it, "fail", run.Verdict("TestSnapKeysTestSuite/TestStale"))
			gotest.Contains(it, after, "=== SNAP TestStale ===\nold\n")
		})
	})
}

func (s *SnapshotKeysTestSuite) TestNames(t *gotest.T) {
	bad := []string{"two\nlines", "=== SNAP forged ==="}
	results := make([]*gotest.R, len(bad))
	for i, name := range bad {
		results[i] = gotest.Record(func(r *gotest.R) { gotest.MatchSnapshot(r, "v", name) })
	}
	written := filepath.Join("testdata", "__snapshots__", "unknown_ext.snap")
	_, err := os.Stat(written)
	// Whatever a regression wrote beside this suite; Remove leaves a dir
	// that holds anything else.
	for _, p := range []string{written, filepath.Dir(written), "testdata"} {
		_ = os.Remove(p)
	}
	t.When("a snapshot name spans lines or spells a section header", func(w *gotest.T) {
		w.It("is refused before anything is written", func(it *gotest.T) {
			for i, m := range results {
				gotest.True(it, m.Failed(), "name %q was accepted", bad[i])
				gotest.Contains(it, m.Message(), "snapshot name")
			}
			gotest.True(it, os.IsNotExist(err), "a refused name still wrote a file")
		})
	})
}

func (s *SnapshotKeysTestSuite) TestSectionsNoTestRead(t *gotest.T) {
	said := func(run *testkit.Run) string {
		var out strings.Builder
		for _, ev := range run.Events {
			out.WriteString(ev.Output)
		}
		return out.String() + run.Stderr
	}
	baseline := filepath.Join("..", "testdata", "snapunread", "testdata", "__snapshots__", "TestUnreadTestSuite.snap")
	before := readFile(t, baseline)
	t.When("a suite that ran whole and passed under GOTEST_CI leaves a section of its snapshot file unread", func(w *gotest.T) {
		run, err := testkit.RunPackageWithEnv(w.Context(), w.TempDir(), []string{"GOTEST_CI=1"}, "../testdata/snapunread/")
		gotest.NoError(w, err)
		w.It("names the file and the section in one line, and leaves the file and the verdict as they are", func(it *gotest.T) {
			gotest.Equal(it, 0, run.Exit, "stdout: %s\nstderr: %s", run.Stdout, run.Stderr)
			gotest.Contains(it, said(&run), "TestUnreadTestSuite.snap holds 1 section no test of this run read: [TestGone]; delete them if their tests are gone")
			gotest.Equal(it, before, readFile(it, baseline))
		})
	})
	t.When("-run names one method of the suite", func(w *gotest.T) {
		run, err := testkit.RunPackageWithEnv(w.Context(), w.TempDir(), []string{"GOTEST_CI=1"}, "../testdata/snapunread/", "-run=^TestUnreadTestSuite$/^TestKept$")
		gotest.NoError(w, err)
		w.It("names no unread section", func(it *gotest.T) {
			gotest.Equal(it, 0, run.Exit, "stdout: %s\nstderr: %s", run.Stdout, run.Stderr)
			gotest.NotContains(it, said(&run), "no test of this run read")
		})
	})
	t.When("a method of a suite that ran whole and passed under GOTEST_CI skips itself before it reads its section", func(w *gotest.T) {
		run, err := testkit.RunPackageWithEnv(w.Context(), w.TempDir(), []string{"GOTEST_CI=1", "SNAP_SKIP=1"}, "../testdata/snapunread/")
		gotest.NoError(w, err)
		w.It("names the sections no test read but not that method's", func(it *gotest.T) {
			gotest.Equal(it, 0, run.Exit, "stdout: %s\nstderr: %s", run.Stdout, run.Stderr)
			gotest.Equal(it, "skip", run.Verdict("TestUnreadTestSuite/TestGated"))
			gotest.Contains(it, said(&run), "TestUnreadTestSuite.snap holds 1 section no test of this run read: [TestGone];")
		})
	})
}

// readFile reads a file of a staged fixture, failing t when it cannot.
func readFile(t *gotest.T, path string) string {
	data, err := os.ReadFile(path) //nolint:gosec // G304: the staged copy of the repo's own fixture
	gotest.NoError(t, err)
	return string(data)
}

func (s *SnapshotKeysTestSuite) TestLegacyKeys(t *gotest.T) {
	root, err := testkit.RepoRoot()
	gotest.NoError(t, err)
	staged := t.TempDir()
	pkg, err := testkit.StagePackage(root, filepath.Join("..", "testdata", "snaplegacy"), staged, "snaplegacymod")
	gotest.NoError(t, err)
	baseline := filepath.Join(pkg, "testdata", "__snapshots__", "TestSnapLegacyTestSuite_ext.snap")
	before := readFile(t, baseline)
	ci, err := testkit.RunCLIWithEnv(t.Context(), staged, []string{"GOTEST_CI=1"}, "-json", "./snaplegacy/")
	gotest.NoError(t, err)
	afterCI := readFile(t, baseline)
	local, err := testkit.RunCLIWithEnv(t.Context(), staged, nil, "-json", "./snaplegacy/")
	gotest.NoError(t, err)
	afterLocal := readFile(t, baseline)
	again, err := testkit.RunCLIWithEnv(t.Context(), staged, []string{"GOTEST_CI=1"}, "-json", "./snaplegacy/")
	gotest.NoError(t, err)
	verdict := func(r testkit.Run, name string) string { return r.Verdict("TestSnapLegacyTestSuite/" + name) }
	t.When("a baseline is keyed the way the release before exact names keyed it", func(w *gotest.T) {
		w.It("its content is compared under the new key", func(it *gotest.T) {
			gotest.Equal(it, "pass", verdict(ci, "TestIssue/issue_#42"), "stdout: %s", ci.Stdout)
			gotest.Equal(it, "pass", verdict(ci, "TestRows/#0"))
			gotest.Equal(it, "fail", verdict(ci, "TestStale/issue_#7"))
			gotest.Equal(it, "fail", verdict(local, "TestStale/issue_#7"))
		})
		w.It("under GOTEST_CI the file is left as it was", func(it *gotest.T) {
			gotest.Equal(it, before, afterCI)
		})
		w.It("otherwise a matching section is renamed to the new key", func(it *gotest.T) {
			gotest.Contains(it, afterLocal, "=== SNAP TestIssue/issue_#42 ===\nforty-two\n")
			gotest.Contains(it, afterLocal, "=== SNAP TestRows/#0 n ===\nrow zero\n")
			gotest.NotContains(it, afterLocal, "=== SNAP TestIssue/issue_ ===")
			gotest.NotContains(it, afterLocal, "=== SNAP TestRows/ n ===")
			gotest.Equal(it, "pass", verdict(again, "TestIssue/issue_#42"), "stdout: %s", again.Stdout)
			gotest.Equal(it, "pass", verdict(again, "TestRows/#0"))
		})
	})
	t.When("the old key of a section is the exact key of another test", func(w *gotest.T) {
		w.It("the section is not reused for a second key", func(it *gotest.T) {
			gotest.Equal(it, "pass", verdict(ci, "TestTwins/x"))
			gotest.Equal(it, "fail", verdict(ci, "TestTwins/x#01"), "x#01 read the section x claims")
			gotest.Equal(it, "pass", verdict(local, "TestTwins/x#01"))
			gotest.Contains(it, afterLocal, "=== SNAP TestTwins/x ===\nfirst twin\n=== SNAP TestTwins/x#01 ===\nsecond twin\n")
		})
	})
}
