package cli_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/mvrahden/go-test/internal/testkit"
	"github.com/mvrahden/go-test/pkg/gotest"
)

// CLIRecordTrustTestSuite: a record holds a package as passed and current
// only for a verdict of all its tests under a mode that covers the run's.
type CLIRecordTrustTestSuite struct{}

func (s *CLIRecordTrustTestSuite) SuiteConfig() gotest.SuiteConfig {
	return gotest.SuiteConfig{Parallel: true, Timeout: 2 * time.Minute} // up to seven runs of a staged module
}

// stageRecords stages testdata/records with a gotest cache of its own.
func stageRecords(t *gotest.T) freshTree {
	root, err := testkit.RepoRoot()
	gotest.NoError(t, err)
	tr := freshTree{dir: filepath.Join(t.TempDir(), "records"), cache: t.TempDir()}
	gotest.NoError(t, testkit.StageModule(root, filepath.Join("..", "testdata", "records"), tr.dir))
	return tr
}

// replace rewrites every old in the tree's file rel to with; old must be there.
func (tr freshTree) replace(t *gotest.T, rel, old, with string) {
	path := filepath.Join(tr.dir, filepath.FromSlash(rel))
	before, err := os.ReadFile(path)
	gotest.NoError(t, err)
	gotest.Contains(t, string(before), old)
	gotest.NoError(t, os.WriteFile(path, []byte(strings.ReplaceAll(string(before), old, with)), 0o600))
}

// ranOf are those of pkgs a text run printed an ok or FAIL line of.
func ranOf(stdout string, pkgs ...string) []string {
	var out []string
	for _, pkg := range pkgs {
		for line := range strings.Lines(stdout) {
			if strings.HasPrefix(line, "ok  \t"+pkg+"\t") || strings.HasPrefix(line, "FAIL\t"+pkg+"\t") {
				out = append(out, pkg)
				break
			}
		}
	}
	return out
}

// trustRecord is the part of last.json these suites read.
type trustRecord struct {
	Manifest struct{ Args []string }
	Result   struct{ ExitCode int }
	Failed   []struct{ Package, Test string }
	Packages []trustPackage
}

type trustPackage struct {
	Package, Verdict string
	Narrowed         bool
	Mode             *struct{ Policy, Short bool }
}

// lastFile is the one runs/*/last.json of the tree's cache.
func (tr freshTree) lastFile(t *gotest.T) string {
	files, err := filepath.Glob(filepath.Join(tr.cache, "runs", "*", "last.json"))
	gotest.NoError(t, err)
	gotest.Len(t, files, 1)
	return files[0]
}

// record reads the one last.json of the tree's cache.
func (tr freshTree) record(t *gotest.T) trustRecord {
	data, err := os.ReadFile(tr.lastFile(t))
	gotest.NoError(t, err)
	var rec trustRecord
	gotest.NoError(t, json.Unmarshal(data, &rec))
	return rec
}

// of is pkg's entry; the zero value when absent.
func (r trustRecord) of(pkg string) trustPackage {
	for _, p := range r.Packages {
		if p.Package == pkg {
			return p
		}
	}
	return trustPackage{}
}

// stripModes rewrites last.json without any package's Mode.
func (tr freshTree) stripModes(t *gotest.T) {
	file := tr.lastFile(t)
	data, err := os.ReadFile(file)
	gotest.NoError(t, err)
	var rec map[string]any
	gotest.NoError(t, json.Unmarshal(data, &rec))
	pkgs, _ := rec["Packages"].([]any)
	gotest.NotEmpty(t, pkgs)
	for _, p := range pkgs {
		entry, _ := p.(map[string]any)
		delete(entry, "Mode")
	}
	out, err := json.Marshal(rec)
	gotest.NoError(t, err)
	gotest.NoError(t, os.WriteFile(file, out, 0o600))
}

const (
	emptyOf1  = "gotest: EMPTY exit 0 | packages: 0 passed, 0 failed, 0 skipped | changed only: 0 ran, 1 unchanged"
	otherMode = " unchanged package(s) run again: the record does not hold their verdict under this run's -short and --ci\n"
)

func (s *CLIRecordTrustTestSuite) TestTheModeInARecord(t *gotest.T) {
	strict, plain := stageRecords(t), stageRecords(t)
	gotest.Equal(t, 0, strict.run(t, "--ci", "-short", "./good/").Exit)
	gotest.Equal(t, 0, plain.run(t, "./good/").Exit)
	on, off := strict.record(t).of("records/good").Mode, plain.record(t).of("records/good").Mode

	t.When("a --ci -short run of a package ends", func(w *gotest.T) {
		w.It("its record lists the package with a Mode whose Policy and Short are on", func(it *gotest.T) {
			gotest.NotZero(it, on)
			gotest.Equal(it, struct{ Policy, Short bool }{true, true}, *on)
		})
		w.It("the record of a plain run lists it with both off", func(it *gotest.T) {
			gotest.NotZero(it, off)
			gotest.Equal(it, struct{ Policy, Short bool }{}, *off)
		})
	})
}

func (s *CLIRecordTrustTestSuite) TestAfterAShortRun(t *gotest.T) {
	tr := stageRecords(t)
	gotest.Equal(t, 0, tr.run(t, "-short", "./brief/").Exit)
	short := tr.run(t, "-short", "--changed", "./brief/")
	plain := tr.run(t, "--changed", "./brief/")

	t.When("--changed follows a -short run of a package unchanged since", func(w *gotest.T) {
		w.It("a plain --changed run runs the package again, saying on stderr that the record does not hold its verdict under this run's -short and --ci", func(it *gotest.T) {
			gotest.Equal(it, 1, plain.Exit, "stderr: %s", plain.Stderr)
			gotest.Equal(it, []string{"records/brief"}, ranOf(plain.Stdout, "records/brief"), "stdout: %s", plain.Stdout)
			gotest.Contains(it, plain.Stderr, "gotest: --changed: 1"+otherMode)
			gotest.Equal(it, "gotest: FAIL exit 1 | packages: 0 passed, 1 failed, 0 skipped | changed only: 1 ran, 0 unchanged", lastLine(plain.Stderr))
		})
		w.It("a -short --changed run leaves it out", func(it *gotest.T) {
			gotest.Equal(it, 0, short.Exit, "stderr: %s", short.Stderr)
			gotest.Equal(it, emptyOf1, lastLine(short.Stderr))
			gotest.NotContains(it, short.Stderr, "run again")
		})
	})
}

func (s *CLIRecordTrustTestSuite) TestCIModeAndAPlainRun(t *gotest.T) {
	loose, strict := stageRecords(t), stageRecords(t)
	gotest.Equal(t, 0, loose.run(t, "./idle/").Exit)
	ci := loose.run(t, "--ci", "--changed", "./idle/")
	gotest.Equal(t, 0, strict.run(t, "--ci", "./good/").Exit)
	plain := strict.run(t, "--changed", "./good/")

	t.When("--changed follows a run of a package unchanged since, one of the two under --ci", func(w *gotest.T) {
		w.It("after a plain run, --ci --changed runs the package again and fails on its test that passed without an assertion", func(it *gotest.T) {
			gotest.Equal(it, 1, ci.Exit, "stderr: %s", ci.Stderr)
			gotest.Contains(it, ci.Stderr, "FAIL: 1 test(s) passed without an assertion")
			gotest.Contains(it, ci.Stderr, "  records/idle TestIdleTestSuite/TestNothing\n")
			gotest.Contains(it, lastLine(ci.Stderr), "| changed only: 1 ran, 0 unchanged")
		})
		w.It("after a --ci run, a plain --changed run leaves the package out", func(it *gotest.T) {
			gotest.Equal(it, 0, plain.Exit, "stderr: %s", plain.Stderr)
			gotest.Equal(it, emptyOf1, lastLine(plain.Stderr))
		})
	})
}

func (s *CLIRecordTrustTestSuite) TestARecordWithoutModes(t *gotest.T) {
	tr := stageRecords(t)
	gotest.Equal(t, 0, tr.run(t, "./good/", "./sound/").Exit)
	tr.stripModes(t)
	first := tr.run(t, "--changed", "./good/", "./sound/")
	second := tr.run(t, "--changed", "./good/", "./sound/")

	t.When("--changed reads a record whose packages carry no mode", func(w *gotest.T) {
		w.It("runs every package given once, saying so on stderr", func(it *gotest.T) {
			gotest.Equal(it, 0, first.Exit, "stderr: %s", first.Stderr)
			gotest.Equal(it, []string{"records/good", "records/sound"}, ranOf(first.Stdout, "records/good", "records/sound"), "stdout: %s", first.Stdout)
			gotest.Contains(it, first.Stderr, "gotest: --changed: 2"+otherMode)
		})
		w.It("leaves them out on the next --changed run", func(it *gotest.T) {
			gotest.Equal(it, "gotest: EMPTY exit 0 | packages: 0 passed, 0 failed, 0 skipped | changed only: 0 ran, 2 unchanged", lastLine(second.Stderr))
		})
	})
}

// fixBad makes both methods of records/bad pass.
func fixBad(t *gotest.T, tr freshTree) {
	tr.replace(t, "bad/bad_suite_test.go", "gotest.Equal(t, 1, 2)", "gotest.Equal(t, 1, 1)")
	tr.replace(t, "bad/bad_suite_test.go", "gotest.Equal(t, 2, 1)", "gotest.Equal(t, 2, 2)")
}

func (s *CLIRecordTrustTestSuite) TestARunNarrowedByFocus(t *gotest.T) {
	tr := stageRecords(t)
	tr.replace(t, "good/good_suite_test.go", "TestHolds", "F_TestHolds")
	gotest.Equal(t, 0, tr.run(t, "./good/", "./bad/").Exit)
	tr.replace(t, "good/good_suite_test.go", "F_TestHolds", "TestHolds")
	changed := tr.run(t, "--changed", "./good/", "./bad/")

	t.When("--changed follows a run that a focused method in one package narrowed, the focus removed since", func(w *gotest.T) {
		w.It("runs again the other package, whose failing test the focus kept from running, and exits 1", func(it *gotest.T) {
			gotest.Equal(it, 1, changed.Exit, "stderr: %s", changed.Stderr)
			gotest.Equal(it, []string{"records/bad", "records/good"}, ranOf(changed.Stdout, "records/bad", "records/good"), "stdout: %s", changed.Stdout)
			gotest.Equal(it, "gotest: FAIL exit 1 | packages: 1 passed, 1 failed, 0 skipped | changed only: 2 ran, 0 unchanged", lastLine(changed.Stderr))
		})
	})
}

func (s *CLIRecordTrustTestSuite) TestAFailedRunGivenPackages(t *gotest.T) {
	tr := stageRecords(t)
	gotest.Equal(t, 1, tr.run(t, "./sound/", "./bad/").Exit)
	tr.replace(t, "sound/sound_suite_test.go", "gotest.Equal(t, 1, 1)", "gotest.Equal(t, 1, 2)")
	fixBad(t, tr)
	gotest.Equal(t, 0, tr.run(t, "--failed", "./sound/", "./bad/").Exit)
	changed := tr.run(t, "--changed", "./sound/", "./bad/")

	t.When("--changed follows a --failed run given two packages, the one that had not failed broken since", func(w *gotest.T) {
		w.It("runs that package again and exits 1", func(it *gotest.T) {
			gotest.Equal(it, 1, changed.Exit, "stderr: %s", changed.Stderr)
			gotest.Contains(it, changed.Stdout, "FAIL\trecords/sound\t")
			gotest.Equal(it, "gotest: FAIL exit 1 | packages: 1 passed, 1 failed, 0 skipped | changed only: 2 ran, 0 unchanged", lastLine(changed.Stderr))
		})
	})
}

func (s *CLIRecordTrustTestSuite) TestFlagsThatLeaveNothingToRun(t *gotest.T) {
	t.When("--changed follows a passing run of a package with a failing test given -count=0, or -test.run, -test.count=0 or -h after -args", func(w *gotest.T) {
		w.It("each such run's record marks the package's verdict as of a part", func(it *gotest.T) {
			for sub, r := range gotest.Each(it, []struct {
				Desc string
				args []string
			}{
				{"-count=0", []string{"-count=0", "./bad/"}},
				{"-test.run after -args", []string{"./bad/", "-args", "-test.run=NoSuchTest"}},
				{"-test.count=0 after -args", []string{"./bad/", "-args", "-test.count=0"}},
				{"-h after -args", []string{"./bad/", "-args", "-h"}},
			}) {
				tr := stageRecords(sub)
				run := tr.run(sub, r.args...)
				gotest.Equal(sub, 0, run.Exit, "stderr: %s", run.Stderr)
				p := tr.record(sub).of("records/bad")
				gotest.Equal(sub, "pass", p.Verdict)
				gotest.True(sub, p.Narrowed)
			}
		})
		w.It("--changed after the -count=0 run runs the package again and exits 1", func(it *gotest.T) {
			tr := stageRecords(it)
			gotest.Equal(it, 0, tr.run(it, "-count=0", "./bad/").Exit)
			changed := tr.run(it, "--changed", "./bad/")
			gotest.Equal(it, 1, changed.Exit, "stderr: %s", changed.Stderr)
			gotest.Equal(it, "gotest: FAIL exit 1 | packages: 0 passed, 1 failed, 0 skipped | changed only: 1 ran, 0 unchanged", lastLine(changed.Stderr))
		})
	})
}

func (s *CLIRecordTrustTestSuite) TestPackagesThatRunNothingByTheirOwnSource(t *gotest.T) {
	tr := stageRecords(t)
	pkgs := []string{"./excluded/", "./benches/", "./plain/", "./good/"}
	gotest.Equal(t, 0, tr.run(t, pkgs...).Exit)
	rec := tr.record(t)
	changed := tr.run(t, append([]string{"--changed"}, pkgs...)...)

	t.When("--changed follows a run of a package whose methods are all excluded, one holding only benchmarks, one without test files and one that passed", func(w *gotest.T) {
		w.It("leaves all four out as unchanged", func(it *gotest.T) {
			gotest.Equal(it, 0, changed.Exit, "stderr: %s", changed.Stderr)
			gotest.Equal(it, "gotest: EMPTY exit 0 | packages: 0 passed, 0 failed, 0 skipped | changed only: 0 ran, 4 unchanged", lastLine(changed.Stderr))
			gotest.False(it, rec.of("records/excluded").Narrowed)
			gotest.False(it, rec.of("records/benches").Narrowed)
		})
	})
}

func (s *CLIRecordTrustTestSuite) TestAPassWithoutAnAssertionThatFailedACIRun(t *gotest.T) {
	tr := stageRecords(t)
	gotest.Equal(t, 1, tr.run(t, "--ci", "./idle/", "./good/").Exit)
	rec := tr.record(t)
	changed := tr.run(t, "--ci", "--changed", "./idle/", "./good/")

	t.When("--ci --changed follows a --ci run that failed only on a test that passed without an assertion", func(w *gotest.T) {
		w.It("the first run's record lists that test's package as failed", func(it *gotest.T) {
			gotest.Equal(it, "fail", rec.of("records/idle").Verdict)
			gotest.Equal(it, "pass", rec.of("records/good").Verdict)
		})
		w.It("--changed runs that package again, leaves the other out and exits 1", func(it *gotest.T) {
			gotest.Equal(it, 1, changed.Exit, "stderr: %s", changed.Stderr)
			gotest.Equal(it, "gotest: FAIL exit 1 | tests: 1 vacuous | packages: 1 passed, 0 failed, 0 skipped | changed only: 1 ran, 1 unchanged", lastLine(changed.Stderr))
		})
	})
}

func (s *CLIRecordTrustTestSuite) TestAPackageAFailedRunDidNotCount(t *gotest.T) {
	tr := stageRecords(t)
	pkgs := []string{"./nomrun/", "./bad/", "./good/"}
	gotest.Equal(t, 1, tr.run(t, pkgs...).Exit)
	fixBad(t, tr)
	first := tr.run(t, append([]string{"--changed"}, pkgs...)...)
	second := tr.run(t, append([]string{"--changed"}, pkgs...)...)

	t.When("--changed follows a run that a test failed, beside a package whose binary ran none of its tests, the failing test fixed since", func(w *gotest.T) {
		w.It("runs both again, leaves out the third that passed, and exits 2, naming the test that never ran", func(it *gotest.T) {
			gotest.Equal(it, 2, first.Exit, "stderr: %s", first.Stderr)
			gotest.Contains(it, first.Stderr, "FAIL: census: 1 declared test(s) never ran\n  records/nomrun TestNoMRunTestSuite/TestNeverRuns\n")
			gotest.Equal(it, "gotest: FAIL exit 2 | tests: 1 not run | packages: 1 passed, 1 failed, 0 skipped | changed only: 2 ran, 1 unchanged", lastLine(first.Stderr))
		})
		w.It("a further --changed run runs that package alone and exits 2", func(it *gotest.T) {
			gotest.Equal(it, 2, second.Exit, "stderr: %s", second.Stderr)
			gotest.Equal(it, "gotest: FAIL exit 2 | tests: 1 not run | packages: 0 passed, 1 failed, 0 skipped | changed only: 1 ran, 2 unchanged", lastLine(second.Stderr))
		})
	})
}

func (s *CLIRecordTrustTestSuite) TestATeardownFailureAlone(t *gotest.T) {
	tr := stageRecords(t)
	gotest.Equal(t, 1, tr.run(t, "./leak/", "./good/").Exit)
	changed := tr.run(t, "--changed", "./leak/", "./good/")

	t.When("--changed follows a run that only a shared fixture's teardown failed", func(w *gotest.T) {
		w.It("runs both packages of that run again and exits 1", func(it *gotest.T) {
			gotest.Equal(it, 1, changed.Exit, "stderr: %s", changed.Stderr)
			gotest.Contains(it, changed.Stderr, "shared fixture teardown")
			gotest.Equal(it, "gotest: FAIL exit 1 | packages: 2 passed, 0 failed, 0 skipped | changed only: 2 ran, 0 unchanged", lastLine(changed.Stderr))
		})
	})
}

func (s *CLIRecordTrustTestSuite) TestATeardownFailureBesideAFailedTest(t *gotest.T) {
	tr := stageRecords(t)
	pkgs := []string{"./leak/", "./good/", "./bad/"}
	gotest.Equal(t, 1, tr.run(t, pkgs...).Exit)
	fixBad(t, tr)
	changed := tr.run(t, append([]string{"--changed"}, pkgs...)...)

	t.When("--changed follows a run that a shared fixture's teardown and a test failed, the test fixed since", func(w *gotest.T) {
		w.It("runs all three packages of that run again and exits 1 on the teardown", func(it *gotest.T) {
			gotest.Equal(it, 1, changed.Exit, "stderr: %s", changed.Stderr)
			gotest.Contains(it, changed.Stderr, "shared fixture teardown")
			gotest.Equal(it, "gotest: FAIL exit 1 | packages: 3 passed, 0 failed, 0 skipped | changed only: 3 ran, 0 unchanged", lastLine(changed.Stderr))
		})
	})
}

func (s *CLIRecordTrustTestSuite) TestACoverageMinimumThatFailed(t *gotest.T) {
	tr := stageRecords(t)
	gotest.Equal(t, 1, tr.run(t, "--min", "90", "./half/").Exit)
	changed := tr.run(t, "--min", "90", "--changed", "./half/")

	t.When("--min --changed follows a run that only its coverage minimum failed", func(w *gotest.T) {
		w.It("runs the package again and exits 1 on the minimum", func(it *gotest.T) {
			gotest.Equal(it, 1, changed.Exit, "stderr: %s", changed.Stderr)
			gotest.Contains(it, changed.Stderr, "FAIL: 50.0% coverage (minimum 90%)\n")
			gotest.Equal(it, "gotest: FAIL exit 1 | packages: 1 passed, 0 failed, 0 skipped | changed only: 1 ran, 0 unchanged", lastLine(changed.Stderr))
		})
	})
}

func (s *CLIRecordTrustTestSuite) TestAChangedRunThatRanNothing(t *gotest.T) {
	tr := stageRecords(t)
	gotest.Equal(t, 1, tr.run(t, "./bad/", "./good/").Exit)
	before := tr.record(t)
	gotest.Equal(t, 0, tr.run(t, "--changed", "./good/").Exit)
	after := tr.record(t)

	t.When("a --changed run given only an unchanged package follows a run that failed another package", func(w *gotest.T) {
		w.It("the record keeps the failed run's exit code, arguments and failures, and the history holds both runs", func(it *gotest.T) {
			gotest.Equal(it, 1, after.Result.ExitCode)
			gotest.Equal(it, []string{"./bad/", "./good/"}, after.Manifest.Args)
			gotest.NotEmpty(it, after.Failed)
			gotest.Equal(it, before.Failed, after.Failed)
			gotest.Len(it, tr.history(it), 2)
		})
	})
}
