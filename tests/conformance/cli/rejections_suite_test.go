package cli_test

import (
	"strings"

	"github.com/mvrahden/go-test/internal/testkit"
	"github.com/mvrahden/go-test/pkg/gotest"
)

// CLIRejectionsTestSuite: a suite the generator refuses is refused once,
// with every reason in one error.
type CLIRejectionsTestSuite struct{}

func (s *CLIRejectionsTestSuite) SuiteConfig() gotest.SuiteConfig {
	// No deadline: this suite compiles code, so its time is not a verdict.
	return gotest.SuiteConfig{Parallel: true, Timeout: gotest.NoDeadline}
}

type rejectionRow struct {
	Desc     string
	pkg      string
	at       string // file:line of the first offending call
	hooks    string // the hooks the error names, in source order
	wrappers string
}

type acceptedRow struct {
	Desc string
	pkg  string
	test string // the generated test the run passes
}

func (s *CLIRejectionsTestSuite) TestHooksCallingT(t *gotest.T) {
	t.When("several hooks call t.T() where their wrapper makes it nil", func(w *gotest.T) {
		w.It("fails with exit 2 and one error on stderr that names every such hook in source order, located at the first", func(it *gotest.T) {
			for sub, row := range gotest.Each(it, []rejectionRow{
				{Desc: "fuzz methods", pkg: "../testdata/clihookcalls/", at: "hookcalls_suite_test.go:14:", hooks: "AfterAll and BeforeAll call t.T()", wrappers: "fuzz"},
				{Desc: "benchmark methods", pkg: "../testdata/clihookbench/", at: "hookbench_suite_test.go:11:", hooks: "AfterAll and BeforeEach call t.T()", wrappers: "benchmark"},
				{Desc: "fuzz and benchmark methods", pkg: "../testdata/clihookmixed/", at: "hookmixed_suite_test.go:10:", hooks: "BeforeEach and BeforeAll call t.T()", wrappers: "benchmark"},
			}) {
				run, err := testkit.RunCLI(sub.Context(), ".", row.pkg)
				gotest.NoError(sub, err)
				gotest.Equal(sub, 2, run.Exit, "stdout: %s\nstderr: %s", run.Stdout, run.Stderr)
				gotest.Equal(sub, 1, strings.Count(run.Stderr, "t.T(), which is nil"), "stderr: %s", run.Stderr)
				gotest.Contains(sub, run.Stderr, row.at)
				gotest.Contains(sub, run.Stderr, "has "+row.wrappers+" methods but "+row.hooks+", which is nil under them")
			}
		})
	})

	t.When("a hook reaches t.T() through parentheses, an alias of t or a method value", func(w *gotest.T) {
		w.It("refuses it with exit 2 like a direct call", func(it *gotest.T) {
			run, err := testkit.RunCLI(it.Context(), ".", "../testdata/clihookshapes/")
			gotest.NoError(it, err)
			gotest.Equal(it, 2, run.Exit, "stdout: %s\nstderr: %s", run.Stdout, run.Stderr)
			for sub, row := range gotest.Each(it, []rejectionRow{
				{Desc: "parentheses", at: "hookshapes_suite_test.go:13:", hooks: "ParenTestSuite"},
				{Desc: "alias", at: "hookshapes_suite_test.go:21:", hooks: "AliasTestSuite"},
				{Desc: "method value", at: "hookshapes_suite_test.go:30:", hooks: "MethodValueTestSuite"},
			}) {
				gotest.Contains(sub, run.Stderr, row.at)
				gotest.Contains(sub, run.Stderr, "suite "+row.hooks+" has fuzz methods but BeforeAll calls t.T()")
			}
		})
	})

	t.When("a fuzz suite's BeforeEach and AfterEach call t.T()", func(w *gotest.T) {
		w.It("runs the suite, each input with a real *testing.T", func(it *gotest.T) {
			passes(it, acceptedRow{pkg: "../testdata/clihookeach/", test: "FuzzHookEachTestSuite_FuzzOne"})
		})
	})

	t.When("a hook checks t.T() for nil before it uses it", func(w *gotest.T) {
		w.It("runs the suite", func(it *gotest.T) {
			passes(it, acceptedRow{pkg: "../testdata/clihookguard/", test: "FuzzHookGuardTestSuite_FuzzOne"})
		})
	})
}

// passes runs row's package and checks the run passes row's test.
func passes(t *gotest.T, row acceptedRow) {
	run, err := testkit.RunCLI(t.Context(), ".", "-json", row.pkg)
	gotest.NoError(t, err)
	gotest.Equal(t, 0, run.Exit, "stdout: %s\nstderr: %s", run.Stdout, run.Stderr)
	var passed []string
	for _, e := range run.Events {
		if e.Action == "pass" && e.Test != "" {
			passed = append(passed, e.Test)
		}
	}
	gotest.Contains(t, passed, row.test)
}
