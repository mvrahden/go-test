package cli_test

import (
	"regexp"
	"strings"

	"github.com/mvrahden/go-test/internal/testkit"
	"github.com/mvrahden/go-test/pkg/gotest"
)

// CLIRejectionsTestSuite: a suite the generator refuses is refused once,
// with every reason in one error.
type CLIRejectionsTestSuite struct{}

func (s *CLIRejectionsTestSuite) SuiteConfig() gotest.SuiteConfig {
	return gotest.SuiteConfig{Parallel: true}
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
				{Desc: "a generic suite another package declares", pkg: "../testdata/clihookalias/", at: "contract.go:11:", hooks: "AfterEach and BeforeAll call t.T()", wrappers: "benchmark"},
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

func (s *CLIRejectionsTestSuite) TestSharedFixtureEachHooks(t *gotest.T) {
	t.When("a shared fixture declares BeforeEach or AfterEach, in the suite's package or another", func(w *gotest.T) {
		w.It("fails with exit 2 and one error on stderr, located at the hook, that says shared fixtures have no per-test hooks and where the work goes instead", func(it *gotest.T) {
			for sub, row := range gotest.Each(it, []struct {
				Desc, pkg, at, fixture, hook string
			}{
				{Desc: "in the suite's package", pkg: "../testdata/clisharedlocal/", at: "fixture.go:12:", fixture: "QueueSharedFixture", hook: "AfterEach"},
				{Desc: "in another package", pkg: "../testdata/clisharedeach/", at: "shared/shared.go:17:", fixture: "CacheSharedFixture", hook: "BeforeEach"},
			}) {
				run, err := testkit.RunCLI(sub.Context(), ".", row.pkg)
				gotest.NoError(sub, err)
				gotest.Equal(sub, 2, run.Exit, "stdout: %s\nstderr: %s", run.Stdout, run.Stderr)
				gotest.Equal(sub, 1, strings.Count(run.Stderr, "shared fixtures have no per-test hooks"), "stderr: %s", run.Stderr)
				msg := `shared fixture "` + row.fixture + `" declares ` + row.hook + `, but shared fixtures have no per-test hooks, so it would never run; do that work in the ` + row.hook + ` of a package fixture that binds ` + row.fixture + `, or in the suite's ` + row.hook
				gotest.Regexp(sub, regexp.QuoteMeta(row.at)+`\d+: `+regexp.QuoteMeta(msg), run.Stderr)
			}
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

func (s *CLIRejectionsTestSuite) TestAGroupThatIsNotAConstant(t *gotest.T) {
	t.When("a suite's ExclusiveGroup is neither a string literal nor a string constant", func(w *gotest.T) {
		w.It("fails with exit 2, located at the value, naming the forms it accepts", func(it *gotest.T) {
			run, err := testkit.RunCLI(it.Context(), ".", "../testdata/cligroupexpr/")
			gotest.NoError(it, err)
			gotest.Equal(it, 2, run.Exit, "stdout: %s\nstderr: %s", run.Stdout, run.Stderr)
			gotest.Contains(it, run.Stderr, "groupexpr_suite_test.go:")
			gotest.Contains(it, run.Stderr, "ExclusiveGroup must be a string literal or a string constant")
		})
	})
}
