package spec_test

import (
	"time"

	"github.com/mvrahden/go-test/internal/testkit"
	"github.com/mvrahden/go-test/pkg/gotest"
)

// SpecSuiteLabelTestSuite: a nested name that ends like a suite or a fixture
// stays a label.
type SpecSuiteLabelTestSuite struct{}

func (s *SpecSuiteLabelTestSuite) SuiteConfig() gotest.SuiteConfig {
	return gotest.SuiteConfig{Parallel: true, Timeout: 60 * time.Second}
}

const suiteLabelFixture = "../testdata/suitelabel/"

func (s *SpecSuiteLabelTestSuite) TestLabelsThatEndLikeASuite(t *gotest.T) {
	live := spec(t, 1, suiteLabelFixture)
	summary, err := testkit.RunCLI(t.Context(), ".", "summary", "--no-color", suiteLabelFixture)
	gotest.NoError(t, err)
	gotest.Equal(t, 1, summary.Exit, "stderr: %s", summary.Stderr)

	t.When("an It's label or a table row's name ends in TestSuite or Fixture", func(w *gotest.T) {
		w.It("a run's spec shows each It whole under its When and the row whole under its method", func(it *gotest.T) {
			gotest.Regexp(it, `(?m)^    when a type is no suite\n      ✗ is told to end its name in TestSuite\n`, live.Stdout)
			gotest.Regexp(it, `(?m)^      ✓ is told to bind a DBFixture$`, live.Stdout)
			gotest.Regexp(it, `(?m)^  Rows \(\S+\)\n    ✓ UserTestSuite$`, live.Stdout)
		})
		w.It("the summary names the failing It whole", func(it *gotest.T) {
			gotest.Contains(it, summary.Stdout, "Names / Labels / when a type is no suite / is told to end its name in TestSuite (")
		})
		w.It("neither counts them as suites", func(it *gotest.T) {
			gotest.Contains(it, live.Stdout, "1 suite, 3 behaviors: 2 passed, 1 failed")
			gotest.Contains(it, summary.Stdout, "1 suite, 3 behaviors: 2 passed, 1 failed")
		})
	})
}
