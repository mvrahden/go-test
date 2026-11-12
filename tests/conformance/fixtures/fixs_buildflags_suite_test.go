package fixtures_test

import (
	"time"

	"github.com/mvrahden/go-test/internal/testkit"
	"github.com/mvrahden/go-test/pkg/gotest"
)

// FixtureBuildFlagsTestSuite: the shared fixture process is built as the
// suites are, with the run's build flags.
type FixtureBuildFlagsTestSuite struct{}

func (s *FixtureBuildFlagsTestSuite) SuiteConfig() gotest.SuiteConfig {
	return gotest.SuiteConfig{Timeout: time.Minute}
}

func (s *FixtureBuildFlagsTestSuite) TestAFixtureBehindABuildTag(t *gotest.T) {
	t.When("a shared fixture is declared in a file behind a build tag and the run sets the tag", func(w *gotest.T) {
		w.It("the fixture comes up and the suite that binds it passes", func(it *gotest.T) {
			for sub, row := range gotest.Each(it, []struct {
				Desc string
				args []string
			}{
				{Desc: "the tag alone", args: []string{"-tags=fixsbuildflags"}},
				{Desc: "the tag beside a flag only go test takes", args: []string{"-tags=fixsbuildflags", "-vet=off"}},
			}) {
				run, err := testkit.RunPackage(sub.Context(), sub.TempDir(), "../testdata/fixsbuildflags/", row.args...)
				gotest.NoError(sub, err)
				gotest.Equal(sub, 0, run.Exit, "stdout: %s\nstderr: %s", run.Stdout, run.Stderr)
				gotest.Equal(sub, "pass", run.Verdict("TestTaggedTestSuite/TestReadsTheFixture"), "stream:\n%s", run.Stdout)
			}
		})
	})
}
