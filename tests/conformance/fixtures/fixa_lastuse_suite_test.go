package fixtures_test

import (
	"time"

	"github.com/mvrahden/go-test/internal/testkit"
	"github.com/mvrahden/go-test/pkg/gotest"
)

// FixtureLastUseTestSuite: a shared fixture is released once no planned
// suite left needs it.
type FixtureLastUseTestSuite struct{}

func (s *FixtureLastUseTestSuite) SuiteConfig() gotest.SuiteConfig {
	cfg := gotest.DefaultSuiteConfig()
	cfg.Parallel = true
	// One run of two suites, the slower 3s; about 15s with the build, twice
	// that cold under -race.
	cfg.Timeout = time.Minute
	return cfg
}

func (s *FixtureLastUseTestSuite) TestReleasedAfterItsLastSuite(t *gotest.T) {
	run, err := testkit.RunPackage(t.Context(), t.TempDir(), "../testdata/fixalastuse/")
	gotest.NoError(t, err)
	t.When("the only suite reading a shared fixture finishes while an unrelated suite still runs", func(w *gotest.T) {
		w.It("releases the fixture before the unrelated suite ends", func(it *gotest.T) {
			gotest.Equal(it, 0, run.Exit, "stdout: %s\nstderr: %s", run.Stdout, run.Stderr)
			gotest.True(it, run.MarkerOrder("fast ran", "a.AfterAll", "slow ends"), "markers: %v", run.Markers)
		})
	})
}

func (s *FixtureLastUseTestSuite) TestSuitesThatNeverRunHoldNothing(t *gotest.T) {
	type row struct {
		Desc     string
		pkgs     []string
		released []string
	}
	for t, r := range gotest.Each(t, []row{
		{
			Desc: "one of its two readers never links", pkgs: []string{"reader", "broken", "slow"},
			released: []string{"reader ran", "a.AfterAll", "slow ends"},
		},
		{Desc: "its only reader never links", pkgs: []string{"broken", "slow"}, released: []string{"a.AfterAll", "slow ends"}},
	}) {
		args := make([]string, 0, len(r.pkgs))
		for _, p := range r.pkgs {
			args = append(args, "../testdata/fixaneverrun/"+p+"/")
		}
		run, err := testkit.RunPackage(t.Context(), t.TempDir(), args[0], args[1:]...)
		gotest.NoError(t, err)
		t.When("a shared fixture's reader is in a package that never links, while an unrelated suite still runs", func(w *gotest.T) {
			w.It("releases the fixture once its readers that can run have finished, before the unrelated suite ends", func(it *gotest.T) {
				gotest.Equal(it, 2, run.Exit, "stdout: %s\nstderr: %s", run.Stdout, run.Stderr)
				gotest.Subset(it, run.Markers, r.released)
				gotest.True(it, run.MarkerOrder(r.released...), "markers: %v", run.Markers)
			})
		})
	}
}

func (s *FixtureLastUseTestSuite) TestADependencyOutlastsItsDependents(t *gotest.T) {
	run, err := testkit.RunPackage(t.Context(), t.TempDir(), "../testdata/fixadeps/")
	gotest.NoError(t, err)
	t.When("the suite binding a shared fixture directly finishes while a suite reading a dependent of it still runs", func(w *gotest.T) {
		w.It("releases the fixture only after its dependent, once neither is needed", func(it *gotest.T) {
			gotest.Equal(it, 0, run.Exit, "stdout: %s\nstderr: %s", run.Stdout, run.Stderr)
			gotest.True(it, run.MarkerOrder("direct ran", "child reader ends", "child.AfterAll", "parent.AfterAll"), "markers: %v", run.Markers)
		})
	})
}

func (s *FixtureLastUseTestSuite) TestATailFixtureComesUpForItsSlot(t *gotest.T) {
	run, err := testkit.RunPackage(t.Context(), t.TempDir(), "../testdata/fixaslots/")
	gotest.NoError(t, err)
	t.When("only the second of two exclusive suites reads a shared fixture", func(w *gotest.T) {
		w.It("starts the fixture after the first suite ran, and releases it after the second", func(it *gotest.T) {
			gotest.Equal(it, 0, run.Exit, "stdout: %s\nstderr: %s", run.Stdout, run.Stderr)
			gotest.True(it, run.MarkerOrder("first ran", "late.BeforeAll", "second ran", "late.AfterAll"), "markers: %v", run.Markers)
		})
	})
}
