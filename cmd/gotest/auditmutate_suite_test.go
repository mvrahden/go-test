package main_test

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	. "github.com/mvrahden/go-test/cmd/gotest"

	"github.com/mvrahden/go-test/internal/gotestaudit"
	"github.com/mvrahden/go-test/internal/gotestspec"
	"github.com/mvrahden/go-test/pkg/gotest"
)

// AuditMutateFlagsTestSuite: what audit's flags set for --mutate and what
// they refuse, and the pieces of a mutant's run that need no process: its
// environment, its limit and the stream it reads.
type AuditMutateFlagsTestSuite struct{}

func (s *AuditMutateFlagsTestSuite) SuiteConfig() gotest.SuiteConfig {
	return gotest.SuiteConfig{Parallel: true}
}

func (s *AuditMutateFlagsTestSuite) TestFlags(t *gotest.T) {
	t.When("--mutate is given with a base, a budget and --parallel", func(w *gotest.T) {
		w.It("runs as --run does, within the budget", func(it *gotest.T) {
			v, err := ExportParseAudit([]string{"--base=main", "--mutate", "--mutate-budget=5m", "--mutate-strings", "--parallel=4"})
			gotest.NoError(it, err)
			gotest.Equal(it, ExportAuditView{Mutate: true, Run: true, Strings: true, Budget: 5 * time.Minute, Parallel: 4}, v)
		})
		w.It("takes three minutes when no budget is given", func(it *gotest.T) {
			v, err := ExportParseAudit([]string{"--base=main", "--mutate"})
			gotest.NoError(it, err)
			gotest.Equal(it, 3*time.Minute, v.Budget)
		})
	})
	for t, tc := range gotest.Each(t, []struct {
		Desc string
		args []string
		err  string
	}{
		{Desc: "a budget without --mutate", args: []string{"--base=main", "--mutate-budget=1m"}, err: "--mutate-budget goes with --mutate"},
		{Desc: "strings without --mutate", args: []string{"--base=main", "--mutate-strings"}, err: "--mutate-strings goes with --mutate"},
		{Desc: "--parallel without a run", args: []string{"--base=main", "--parallel=2"}, err: "--parallel bounds the run --run or --mutate makes; give it with one"},
		{Desc: "survived without --mutate", args: []string{"--base=main", "--run", "--fail-on=survived"}, err: "--fail-on=survived judges mutants: give --mutate, with --base or --diff"},
		{Desc: "--mutate without changed lines", args: []string{"--base-record=b.json", "--mutate"}, err: "--mutate mutates the changed lines: give --base=<ref> or --diff=<file>"},
		{Desc: "a budget that is no duration", args: []string{"--base=main", "--mutate", "--mutate-budget=soon"}, err: `invalid --mutate-budget "soon": give a positive duration such as 3m`},
		{Desc: "a budget of nothing", args: []string{"--base=main", "--mutate", "--mutate-budget=0s"}, err: `invalid --mutate-budget "0s": give a positive duration such as 3m`},
		{Desc: "--parallel of no count", args: []string{"--base=main", "--mutate", "--parallel=0"}, err: `invalid --parallel "0": give a positive count`},
		{Desc: "--mutate on a record", args: []string{"--record", "--mutate"}, err: "--record writes the record of the tree; it does not take --mutate"},
	}) {
		_, err := ExportParseAudit(tc.args)
		gotest.Error(t, err)
		gotest.Equal(t, tc.err, err.Error())
	}
}

func (s *AuditMutateFlagsTestSuite) TestJobs(t *gotest.T) {
	t.When("--parallel is given", func(w *gotest.T) {
		w.It("runs no more mutants at once than it allows and shares it among them", func(it *gotest.T) {
			jobs, per := ExportMutationJobs(1)
			gotest.Equal(it, []int{1, 1}, []int{jobs, per})
			jobs, per = ExportMutationJobs(40)
			gotest.Equal(it, min(4, max(1, runtime.GOMAXPROCS(0)/2)), jobs)
			gotest.Equal(it, 40/jobs, per)
		})
		w.It("leaves the runs' own --parallel when not given", func(it *gotest.T) {
			_, per := ExportMutationJobs(0)
			gotest.Equal(it, 0, per)
		})
	})
}

func (s *AuditMutateFlagsTestSuite) TestEnvironments(t *gotest.T) {
	env := []string{"PATH=/bin", "GOCOVERDIR=/cov", "GOTEST_MUTANT=9", "GOTEST_MUTANT_OVERLAY=/old", "GOTEST_SUITE_COVERAGE=/old"}
	t.When("a run on the mutants is made", func(w *gotest.T) {
		w.It("names the overlay and the mutant, and measures no coverage", func(it *gotest.T) {
			gotest.Equal(it, []string{"PATH=/bin", "GOTEST_MUTANT_OVERLAY=/m/overlay.json", "GOTEST_MUTANT=7"}, ExportRunEnv(env, "/m/overlay.json", 7))
		})
		w.It("switches no mutant on for id 0", func(it *gotest.T) {
			gotest.Equal(it, []string{"PATH=/bin", "GOTEST_MUTANT_OVERLAY=/m/overlay.json"}, ExportRunEnv(env, "/m/overlay.json", 0))
		})
	})
	t.When("the coverage run is made", func(w *gotest.T) {
		w.It("keeps the caller's GOCOVERDIR and names the suites' directory, without the mutants", func(it *gotest.T) {
			gotest.Equal(it, []string{"PATH=/bin", "GOCOVERDIR=/cov", "GOTEST_SUITE_COVERAGE=" + filepath.Join("/m", "suites")}, ExportCoverageEnv(env, "/m"))
		})
	})
}

func (s *AuditMutateFlagsTestSuite) TestTheOverlay(t *gotest.T) {
	t.When("the mutated files lie under a symlinked directory", func(w *gotest.T) {
		if runtime.GOOS == "windows" {
			w.Skipf("symlinks need privileges on Windows")
		}
		real := w.TempDir()
		link := filepath.Join(w.TempDir(), "link")
		gotest.NoError(w, os.Symlink(real, link))
		resolved, err := filepath.EvalSymlinks(real)
		gotest.NoError(w, err)
		gotest.NoError(w, os.WriteFile(filepath.Join(real, "a.go"), []byte("package a\n"), 0o600))
		replace, err := ExportWriteOverlay(w.TempDir(), map[string][]byte{
			filepath.Join(link, "a.go"):     []byte("package a // mutated\n"),
			filepath.Join(link, "added.go"): []byte("package a\n"),
		})
		gotest.NoError(w, err)
		w.It("names each file under the link and under the resolved directory, a file the plan adds too", func(it *gotest.T) {
			for _, base := range []string{"a.go", "added.go"} {
				gotest.Contains(it, replace, filepath.Join(link, base))
				gotest.Contains(it, replace, filepath.Join(resolved, base))
				gotest.Equal(it, replace[filepath.Join(link, base)], replace[filepath.Join(resolved, base)])
			}
		})
	})
}

func (s *AuditMutateFlagsTestSuite) TestTheStreamOfAMutant(t *gotest.T) {
	stream := `{"Action":"run","Package":"m/a","Test":"TestS"}` + "\n" +
		`{"Action":"fail","Package":"m/a","Test":"TestS/TestX","Elapsed":0.1}` + "\n" +
		`not json` + "\n" +
		`{"Action":"fail","Package":"m/a","Test":"TestS","Elapsed":1.5}` + "\n" +
		`{"Action":"fail","Package":"m/a"}` + "\n"
	t.When("a mutant's run fails a test", func(w *gotest.T) {
		w.It("names the first test that failed, a line split across writes too", func(it *gotest.T) {
			by, _, _ := ExportReadStream([]string{stream[:70], stream[70:]}, false)
			gotest.Equal(it, "m/a TestS/TestX", by)
		})
	})
	t.When("the zero run's stream is read", func(w *gotest.T) {
		w.It("lists each failed suite and package and times each suite", func(it *gotest.T) {
			by, failed, took := ExportReadStream([]string{stream}, true)
			gotest.Empty(it, by)
			gotest.Equal(it, []string{"m/a TestS", "m/a"}, failed)
			gotest.Equal(it, map[string]time.Duration{"m/a TestS": 1500 * time.Millisecond}, took)
			gotest.Equal(it, []string{"m/a TestS"}, ExportTestsFirst(failed))
			gotest.Equal(it, []string{"m/a", "gotest run"}, ExportTestsFirst([]string{"m/a", "gotest run"}))
		})
	})
}

func (s *AuditMutateFlagsTestSuite) TestALimit(t *gotest.T) {
	took := map[string]time.Duration{"m/a TestS": 2 * time.Second, "m/b TestT": time.Second}
	t.When("the run with no mutant on took 5s whole, its suites 3s of their own", func(w *gotest.T) {
		w.It("gives a first run three times the whole run and a second, and a second run twice that", func(it *gotest.T) {
			first, second := ExportLimit(took, 5*time.Second, "m/a TestS", "m/b TestT")
			gotest.Equal(it, 16*time.Second, first)
			gotest.Equal(it, 32*time.Second, second)
		})
	})
	t.When("the suites' own time is longer than the whole run's", func(w *gotest.T) {
		w.It("gives a first run three times the suites' time and a second", func(it *gotest.T) {
			first, _ := ExportLimit(took, 2*time.Second, "m/a TestS", "m/b TestT")
			gotest.Equal(it, 10*time.Second, first)
		})
	})
}

func (s *AuditMutateFlagsTestSuite) TestTheCoverageRun(t *gotest.T) {
	dir := t.TempDir()
	suites := filepath.Join(dir, "suites")
	gotest.NoError(t, os.MkdirAll(filepath.Join(suites, "0.covdata"), 0o700))
	gotest.NoError(t, os.MkdirAll(filepath.Join(suites, "1.covdata"), 0o700))
	gotest.NoError(t, os.WriteFile(filepath.Join(suites, "0.out"), []byte("mode: set\nm/a/a.go:3.1,5.2 2 1\nm/a/a.go:8.1,9.2 1 0\n"), 0o600))
	gotest.NoError(t, os.WriteFile(filepath.Join(suites, "1.out"), []byte("mode: set\nm/a/a.go:8.1,8.9 1 1\n"), 0o600))
	index := `[{"package":"m/a","suite":"TestS","profile":"` + filepath.ToSlash(filepath.Join(suites, "0.out")) + `","coverDir":"` + filepath.ToSlash(filepath.Join(suites, "0.covdata")) + `"},` +
		`{"package":"m/b","suite":"TestT","profile":"` + filepath.ToSlash(filepath.Join(suites, "1.out")) + `","coverDir":"` + filepath.ToSlash(filepath.Join(suites, "1.covdata")) + `"}]`
	gotest.NoError(t, os.WriteFile(filepath.Join(suites, "suites.json"), []byte(index), 0o600))
	t.When("the coverage run passed", func(w *gotest.T) {
		w.It("maps each suite to the lines its blocks that ran span", func(it *gotest.T) {
			b, err := ExportReadBaseline(dir, []string{"m/a"}, nil, 0, "", "m/a/a.go")
			gotest.NoError(it, err)
			gotest.Equal(it, ExportBaseline{Passed: true, Suites: []string{"m/a TestS", "m/b TestT"}, Ran: map[string][]int{"m/a TestS": {3, 4, 5}, "m/b TestT": {8}}}, b)
		})
	})
	t.When("the coverage run failed a test", func(w *gotest.T) {
		w.It("names the test and its package, the test first", func(it *gotest.T) {
			events := []gotestspec.TestEvent{
				{Action: gotestspec.ActionFail, Package: "m/a", Test: "TestS/TestX"},
				{Action: gotestspec.ActionFail, Package: "m/a", Test: "TestS"},
				{Action: gotestspec.ActionFail, Package: "m/a"},
			}
			b, err := ExportReadBaseline(dir, []string{"m/a"}, events, 1, "", "m/a/a.go")
			gotest.NoError(it, err)
			gotest.False(it, b.Passed)
			gotest.Equal(it, []string{"m/a TestS"}, b.Failed)
		})
	})
	t.When("a passing run kept no suite apart", func(w *gotest.T) {
		w.It("refuses to judge", func(it *gotest.T) {
			_, err := ExportReadBaseline(it.TempDir(), nil, nil, 0, "", "")
			gotest.ErrorContains(it, err, "--mutate: the run kept no suite's coverage")
		})
	})
}

// AuditMutateRunTestSuite: the mutants of internal/mutate's testdata
// package refused, judged in process through a stand-in CLI that answers
// each mutant's run by its id: 1 fails a test, 2 never ends, 3 exits 1,
// every other passes.
//
// Sequential: it changes directory and sets the environment.
type AuditMutateRunTestSuite struct {
	runner, log string
	files       []gotestaudit.ProdFile
}

func (s *AuditMutateRunTestSuite) SuiteConfig() gotest.SuiteConfig {
	return gotest.SuiteConfig{Timeout: 60 * time.Second}
}

func (s *AuditMutateRunTestSuite) SuiteGuard() string {
	if runtime.GOOS == "windows" {
		return "the stand-in CLI is a shell script"
	}
	return ""
}

// standIn answers a run as the id GOTEST_MUTANT names asks, logging its
// arguments, and stops on a stop line.
const standIn = `#!/bin/sh
echo "${GOTEST_MUTANT:-0} $*" >> "$STANDIN_LOG"
[ -n "$GOTEST_MUTANT_OVERLAY" ] || exit 3
pass='{"Action":"pass","Package":"ops/refused","Test":"TestR","Elapsed":0.01}'
case "${GOTEST_MUTANT:-0}" in
1) echo '{"Action":"fail","Package":"ops/refused","Test":"TestR/TestWarm"}' ;;
2) ;;
3) exit 1 ;;
*) echo "$pass"; exit 0 ;;
esac
while read -r line; do [ "$line" = stop ] && exit 130; done
exit 130
`

func (s *AuditMutateRunTestSuite) BeforeEach(t *gotest.T) {
	dir := t.TempDir()
	s.runner, s.log = filepath.Join(dir, "gotest"), filepath.Join(dir, "calls")
	gotest.NoError(t, os.WriteFile(s.runner, []byte(standIn), 0o700)) //nolint:gosec // G306: an executable
	ops, err := filepath.Abs(filepath.Join("..", "..", "internal", "mutate", "testdata", "ops"))
	gotest.NoError(t, err)
	t.T().Chdir(ops)
	t.Setenv("GOWORK", "off")
	t.Setenv("GOTEST_CACHE_DIR", filepath.Join(dir, "cache"))
	t.Setenv("STANDIN_LOG", s.log)
	file := filepath.Join(ops, "refused", "refused.go")
	lines := make([]int, 40)
	for i := range lines {
		lines[i] = i + 1
	}
	s.files = []gotestaudit.ProdFile{{Module: "ops", File: "refused/refused.go", Package: "ops/refused", Lines: lines, Abs: file}}
}

// mutate judges the files' mutants as ExportMutate does, the log of the
// stand-in's calls emptied first.
func (s *AuditMutateRunTestSuite) mutate(t *gotest.T, files []gotestaudit.ProdFile, budget time.Duration, passed bool, runner string) *gotestaudit.Document {
	gotest.NoError(t, os.RemoveAll(s.log))
	doc, err := ExportMutate(files, budget, "ops/refused TestR", passed, runner)
	gotest.NoError(t, err)
	return doc
}

func (s *AuditMutateRunTestSuite) calls(t *gotest.T) []string {
	data, err := os.ReadFile(s.log)
	if os.IsNotExist(err) {
		return nil
	}
	gotest.NoError(t, err)
	return strings.Split(strings.TrimSpace(string(data)), "\n")
}

func verdictsOf(d *gotestaudit.Document) map[int][]string {
	out := map[int][]string{}
	for i := range d.Mutation.Lines {
		l := &d.Mutation.Lines[i]
		for _, m := range l.Mutants {
			out[l.Line] = append(out[l.Line], strings.TrimSpace(m.Verdict+" "+m.By))
		}
	}
	return out
}

func (s *AuditMutateRunTestSuite) TestJudged(t *gotest.T) {
	doc := s.mutate(t, s.files, time.Minute, true, s.runner)
	t.When("each mutant's run is answered by its id", func(w *gotest.T) {
		w.It("reads a failed test as killed by it, a run past its limit as a timeout and an exit 1 as killed", func(it *gotest.T) {
			v := verdictsOf(doc)
			gotest.Equal(it, []string{"timeout", "killed ops/refused TestR/TestWarm"}, v[13])
			gotest.Equal(it, []string{"killed the run's exit 1"}, v[14])
			c := doc.Mutation.Counts
			gotest.Equal(it, []int{12, 2, 1, 9, 7}, []int{c.Mutants, c.Killed, c.Timeout, c.Survived, c.SurvivedLines})
			gotest.True(it, doc.Mutation.Baseline.Passed)
			gotest.Equal(it, 1, doc.Mutation.Baseline.Suites)
		})
		w.It("runs the suites alone on the mutants' build, live and stoppable, first with none switched on, a timeout twice", func(it *gotest.T) {
			calls := s.calls(it)
			gotest.Len(it, calls, 14)
			gotest.Equal(it, "0 --live --control=stdin -json -run=^(TestR)$ ops/refused", calls[0])
			gotest.Regexp(it, `^2 `, calls[13], "the run past its limit runs again last")
		})
		w.It("notes how the mutants were built and how a survivor is kept", func(it *gotest.T) {
			gotest.Len(it, doc.Notes, 2)
			gotest.Regexp(it, `^--mutate: every mutant is compiled into one build`, doc.Notes[0])
			gotest.Regexp(it, `^--mutate: a survivor no test can tell apart`, doc.Notes[1])
		})
	})
}

func (s *AuditMutateRunTestSuite) TestNotJudged(t *gotest.T) {
	t.When("the budget is spent at once", func(w *gotest.T) {
		w.It("runs the suites with none switched on and judges no mutant", func(it *gotest.T) {
			doc := s.mutate(it, s.files, time.Nanosecond, true, s.runner)
			gotest.Equal(it, 12, doc.Mutation.Counts.NotJudged)
			gotest.Len(it, s.calls(it), 1)
		})
	})
	t.When("the coverage run failed", func(w *gotest.T) {
		w.It("runs nothing and names the failure among what was not compared", func(it *gotest.T) {
			doc := s.mutate(it, s.files, time.Minute, false, s.runner)
			gotest.Equal(it, 12, doc.Mutation.Counts.NotJudged)
			gotest.Empty(it, s.calls(it))
			gotest.Equal(it, []string{"--mutate: no mutant was judged: ops/refused TestR failed before any mutant was switched on"}, doc.Incomplete)
		})
	})
	t.When("the suites fail with no mutant on", func(w *gotest.T) {
		w.It("judges no mutant, naming the run", func(it *gotest.T) {
			failing := filepath.Join(it.TempDir(), "gotest")
			gotest.NoError(it, os.WriteFile(failing, []byte("#!/bin/sh\nexit 1\n"), 0o700)) //nolint:gosec // G306: an executable
			doc := s.mutate(it, s.files, time.Minute, true, failing)
			gotest.False(it, doc.Mutation.Baseline.Passed)
			gotest.Equal(it, []string{"the run (exit 1)"}, doc.Mutation.Baseline.Failed)
			gotest.Equal(it, 12, doc.Mutation.Counts.NotJudged)
		})
	})
	t.When("no production line changed", func(w *gotest.T) {
		w.It("mutates nothing and says so", func(it *gotest.T) {
			doc := s.mutate(it, nil, time.Minute, true, s.runner)
			gotest.Zero(it, doc.Mutation)
			gotest.Equal(it, []string{"--mutate: no changed production line, so nothing was mutated"}, doc.Notes)
		})
	})
}
