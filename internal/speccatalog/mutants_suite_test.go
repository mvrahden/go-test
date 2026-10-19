package speccatalog_test

import (
	"os"
	"path/filepath"

	"github.com/mvrahden/go-test/internal/speccatalog"
	"github.com/mvrahden/go-test/pkg/gotest"
)

const benchSpec = "# Bench Specification\n\n## Purpose\nHow bench compares runs.\n\n## Requirements\n\n" +
	"### Requirement: Gate\nA zero gate SHALL compare without a gate.\n\n" +
	"#### Scenario: A zero gate\n- **WHEN** --gate=0 is given\n- **THEN** compares without a gate\n"

const judgeCatalog = `{"packages":[
 {"path":"github.com/mvrahden/go-test/tests/conformance/cli","nodes":[{"kind":"suite","name":"TestCLIVersionTestSuite","children":[
  {"kind":"method","name":"TestVersion","children":[
   {"kind":"block","vocab":"when","display":"when the version subcommand runs","name":"the_version_subcommand_runs","children":[
    {"kind":"block","vocab":"it","display":"exits 0 and prints the version on stdout","children":[]}]}]}]}]},
 {"path":"github.com/mvrahden/go-test/tests/conformance/bench","nodes":[{"kind":"suite","name":"TestBenchGateTestSuite","children":[
  {"kind":"method","name":"TestZeroGate","children":[
   {"kind":"block","vocab":"when","display":"when --gate=0 is given","name":"--gate=0_is_given","children":[
    {"kind":"block","vocab":"it","display":"compares without a gate","children":[]}]}]}]}]}]}`

const (
	versionClause = "Clause: when the version subcommand runs → exits 0 and prints the version on stdout\n"
	gateClause    = "Clause: when --gate=0 is given → compares without a gate\n"
	cliDiff       = "--- a/cmd/gotest/bench.go\n+++ b/cmd/gotest/bench.go\n"
)

// MutantsTestSuite: a mutant runs only what its Expect lines can match, and a
// patch that cannot be judged says why.
type MutantsTestSuite struct{}

func (s *MutantsTestSuite) SuiteConfig() gotest.SuiteConfig {
	cfg := gotest.DefaultSuiteConfig()
	cfg.Parallel = true
	return cfg
}

func judgeRuns(patch string) ([]speccatalog.JudgeRun, []string) {
	specs := []speccatalog.Spec{
		speccatalog.ParseSpec("cli", "specs/cli/spec.md", []byte(sampleSpec)),
		speccatalog.ParseSpec("bench", "specs/bench/spec.md", []byte(benchSpec)),
	}
	return speccatalog.JudgeRuns(specs, []byte(judgeCatalog), []byte(patch))
}

func (s *MutantsTestSuite) TestJudgeRuns(t *gotest.T) {
	t.When("every Expect line names a conformance failure", func(w *gotest.T) {
		runs, problems := judgeRuns("Expect: ^FAIL .*conformance/cli .* / exits 0\n" + versionClause + gateClause + cliDiff)
		w.It("runs that package alone, selecting the tests that hold the patch's clauses of its capability", func(it *gotest.T) {
			gotest.Empty(it, problems)
			gotest.Equal(it, []speccatalog.JudgeRun{{Packages: []string{"./tests/conformance/cli"}, Tests: []string{"CLIVersionTestSuite/TestVersion"}}}, runs)
			gotest.Equal(it, "^(TestCLIVersionTestSuite)$/^(TestVersion)$", runs[0].RunFilter())
		})
	})
	t.When("a conformance failure is expected of a capability no Clause line names", func(w *gotest.T) {
		runs, problems := judgeRuns("Expect: ^FAIL .*conformance/bench .* / x\n" + versionClause + cliDiff)
		w.It("runs every test of that package", func(it *gotest.T) {
			gotest.Empty(it, problems)
			gotest.Equal(it, []speccatalog.JudgeRun{{Packages: []string{"./tests/conformance/bench"}}}, runs)
			gotest.Zero(it, runs[0].RunFilter())
		})
	})
	t.When("an Expect line names another check and the patch has no Judge: line", func(w *gotest.T) {
		runs, problems := judgeRuns("Expect: failing: exit code 0, want 1\n" + versionClause + cliDiff)
		w.It("runs the ring-0 packages and the canary whole", func(it *gotest.T) {
			gotest.Empty(it, problems)
			gotest.Equal(it, []speccatalog.JudgeRun{{Packages: speccatalog.Judged}}, runs)
		})
	})
	t.When("Judge: lines name the runs for the other Expect lines", func(w *gotest.T) {
		runs, problems := judgeRuns("Expect: failing: exit code 0, want 1\nExpect: ^FAIL .*conformance/cli .* / exits 0\n" +
			"Judge: ./tests/canary CanaryTestSuite/TestA CanaryTimingTestSuite/TestB\nJudge: ./pkg/gotestruntime\n" + versionClause + cliDiff)
		w.It("runs each as written, then the conformance package", func(it *gotest.T) {
			gotest.Empty(it, problems)
			gotest.Equal(it, []speccatalog.JudgeRun{
				{Packages: []string{"./tests/canary"}, Tests: []string{"CanaryTestSuite/TestA", "CanaryTimingTestSuite/TestB"}},
				{Packages: []string{"./pkg/gotestruntime"}, Tests: []string{}},
				{Packages: []string{"./tests/conformance/cli"}, Tests: []string{"CLIVersionTestSuite/TestVersion"}},
			}, runs)
			gotest.Equal(it, "^(TestCanaryTestSuite|TestCanaryTimingTestSuite)$/^(TestA|TestB)$", runs[0].RunFilter())
		})
	})
	t.When("a patch cannot be judged as its header stands", func(w *gotest.T) {
		for t, row := range gotest.Each(w, []struct {
			Name, Patch, Problem string
		}{
			{"a Clause line no scenario states", "Expect: x\nClause: when nothing is stated → is no clause\n" + cliDiff, "Clause line names no clause of specs/: when nothing is stated → is no clause"},
			{"an Alone line without a reason", "Expect: x\nAlone:\n" + cliDiff, "Alone: line gives no reason"},
			{"no Expect line", gateClause + cliDiff, "no Expect: line, name the check that must catch it"},
			{"a Judge line without a package", "Expect: x\nJudge: CanaryTestSuite/TestA\n" + cliDiff, "Judge: line names no package directory: CanaryTestSuite/TestA"},
			{"a Judge line with a bare test name", "Expect: x\nJudge: ./tests/canary TestA\n" + cliDiff, `Judge: line names "TestA", not a <Suite>/<Method>`},
			{"a linked mutant expecting a conformance failure", "Expect: ^FAIL .*conformance/cli x\n--- a/pkg/gotest/t.go\n+++ b/pkg/gotest/t.go\n", "an Expect line names a conformance failure, but the mutant lives in what test binaries link, where the conformance suites assert with the mutated code"},
		}) {
			_, problems := judgeRuns(row.Patch)
			gotest.Equal(t, []string{row.Problem}, problems)
		}
	})
}

func (s *MutantsTestSuite) TestAskedAlone(t *gotest.T) {
	t.When("a patch says why it must run alone", func(w *gotest.T) {
		w.It("runs alone", func(it *gotest.T) {
			gotest.True(it, speccatalog.AskedAlone([]byte("Expect: x\nAlone: the check races the clock\n"+cliDiff)))
		})
	})
	t.When("a patch has no Alone: line, or one only below the diff", func(w *gotest.T) {
		w.It("does not", func(it *gotest.T) {
			gotest.False(it, speccatalog.AskedAlone([]byte("Expect: x\n"+cliDiff)))
			gotest.False(it, speccatalog.AskedAlone([]byte("Expect: x\n"+cliDiff+"Alone: in the diff\n")))
		})
	})
}

func (s *MutantsTestSuite) TestAlone(t *gotest.T) {
	dir := t.TempDir()
	gotest.NoError(t, os.MkdirAll(filepath.Join(dir, "pkg", "testdata", "fixture"), 0o750))
	gotest.NoError(t, os.MkdirAll(filepath.Join(dir, "timed"), 0o750))
	write := func(path, src string) {
		gotest.NoError(t, os.WriteFile(filepath.Join(dir, path), []byte(src), 0o600))
	}
	write("pkg/a_test.go", "package pkg\n\nfunc (s *ShareTestSuite) TestA(t *T) {}\n\n"+
		"func (s *OffTestSuite) SuiteConfig() SuiteConfig { c := SuiteConfig{Exclusive: false}; c.Exclusive = false; return c }\n")
	write("pkg/testdata/fixture/f_test.go", "package fixture\n\nfunc (s *FixtureTestSuite) SuiteConfig() SuiteConfig { return SuiteConfig{Exclusive: true} }\n")
	write("timed/b_test.go", "package timed\n\nfunc (s *ShareTestSuite) TestA(t *T) {}\n\n"+
		"func (s *TimeTestSuite) SuiteConfig() SuiteConfig { c := SuiteConfig{}; c.Exclusive = true; return c }\n\nfunc (s *TimeTestSuite) TestB(t *T) {}\n")
	alone := func(t *gotest.T, run speccatalog.JudgeRun) bool {
		a, problems, err := run.Alone(dir)
		gotest.NoError(t, err)
		gotest.Empty(t, problems)
		return a
	}
	t.When("a run selects every test of packages whose suites leave Exclusive unset or false", func(w *gotest.T) {
		w.It("shares the machine, and the Exclusive suite under testdata is never read", func(it *gotest.T) {
			gotest.False(it, alone(it, speccatalog.JudgeRun{Packages: []string{"./pkg/..."}}))
		})
	})
	t.When("a run selects every test of a package holding an Exclusive suite", func(w *gotest.T) {
		w.It("runs alone", func(it *gotest.T) {
			gotest.True(it, alone(it, speccatalog.JudgeRun{Packages: []string{"./timed"}}))
		})
	})
	t.When("a run selects tests of that package's other suites only", func(w *gotest.T) {
		w.It("shares the machine", func(it *gotest.T) {
			gotest.False(it, alone(it, speccatalog.JudgeRun{Packages: []string{"./timed"}, Tests: []string{"ShareTestSuite/TestA"}}))
		})
	})
	t.When("a run selects a test of the Exclusive suite", func(w *gotest.T) {
		w.It("runs alone", func(it *gotest.T) {
			gotest.True(it, alone(it, speccatalog.JudgeRun{Packages: []string{"./timed"}, Tests: []string{"ShareTestSuite/TestA", "TimeTestSuite/TestB"}}))
		})
	})
	t.When("a run selects a test its package does not declare", func(w *gotest.T) {
		_, problems, err := speccatalog.JudgeRun{Packages: []string{"./pkg"}, Tests: []string{"ShareTestSuite/TestGone"}}.Alone(dir)
		w.It("names it", func(it *gotest.T) {
			gotest.NoError(it, err)
			gotest.Equal(it, []string{"./pkg declares no ShareTestSuite/TestGone"}, problems)
		})
	})
}

func (s *MutantsTestSuite) TestLinksIntoTestBinaries(t *gotest.T) {
	t.When("a patch touches pkg/gotest and cmd/gotest", func(w *gotest.T) {
		patch := "Expect: x\n--- a/cmd/gotest/bench.go\n+++ b/cmd/gotest/bench.go\n@@ -1 +1 @@\n--- a/pkg/gotest/assert.go\n+++ b/pkg/gotest/assert.go\n"
		w.It("links into test binaries", func(it *gotest.T) {
			gotest.True(it, speccatalog.LinksIntoTestBinaries([]byte(patch)))
		})
	})
	t.When("a patch touches only cmd/gotest", func(w *gotest.T) {
		patch := "Expect: x\n--- a/cmd/gotest/bench.go\n+++ b/cmd/gotest/bench.go\n@@ -1 +1 @@\n-x\n+y\n"
		w.It("does not", func(it *gotest.T) {
			gotest.False(it, speccatalog.LinksIntoTestBinaries([]byte(patch)))
		})
	})
}

func (s *MutantsTestSuite) TestAnExplicitBehaviorReplacesTheBroadConformanceRun(t *gotest.T) {
	patch := "Expect: ^FAIL .*conformance/cli .* / exits 0\n" + versionClause +
		"Judge: ./tests/conformance/cli CLIVersionTestSuite/TestVersion/the_version_subcommand_runs\n" + cliDiff
	runs, problems := judgeRuns(patch)
	gotest.Empty(t, problems)
	gotest.Len(t, runs, 1)
	gotest.Equal(t, "^(TestCLIVersionTestSuite)$/^(TestVersion)$/^(the_version_subcommand_runs)$", runs[0].RunFilter())
}

func (s *MutantsTestSuite) TestAnUnknownNestedBehaviorIsRejected(t *gotest.T) {
	_, problems := judgeRuns("Expect: ^FAIL .*conformance/cli .* / exits 0\n" + versionClause +
		"Judge: ./tests/conformance/cli CLIVersionTestSuite/TestVersion/missing\n" + cliDiff)
	gotest.Equal(t, []string{"catalog declares no selected behavior: CLIVersionTestSuite/TestVersion/missing"}, problems)
}

func (s *MutantsTestSuite) TestNestedNamesAreLiteralAndNeverCrossCombined(t *gotest.T) {
	runs, problems := judgeRuns("Expect: failure\nJudge: ./tests/canary CanaryTestSuite/TestA/a[1] CanaryTestSuite/TestB/b(2)\n" + cliDiff)
	gotest.Empty(t, problems)
	gotest.Len(t, runs, 2)
	gotest.Equal(t, `^(TestCanaryTestSuite)$/^(TestA)$/^(a\[1\])$`, runs[0].RunFilter())
	gotest.Equal(t, `^(TestCanaryTestSuite)$/^(TestB)$/^(b\(2\))$`, runs[1].RunFilter())
}
