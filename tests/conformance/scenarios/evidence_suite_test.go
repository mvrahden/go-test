package scenarios_test

import (
	"os"
	"path/filepath"
	"time"

	"github.com/mvrahden/go-test/internal/testkit"
	"github.com/mvrahden/go-test/pkg/gotest"
)

// ScenariosEvidenceTestSuite: a saved stream or a run of its own judges
// each bound scenario.
type ScenariosEvidenceTestSuite struct {
	dir             string
	stream, verify  testkit.Run
	input, verified document
}

const checkoutSpec = "specs/checkout/spec.md"

func (s *ScenariosEvidenceTestSuite) SuiteConfig() gotest.SuiteConfig {
	return gotest.SuiteConfig{Parallel: true, Timeout: 30 * time.Second, SetupTimeout: 90 * time.Second}
}

func (s *ScenariosEvidenceTestSuite) BeforeAll(t *gotest.T) {
	s.dir = tempDir(t, "gotest-scenarios-evidence-")
	mod := stage(t, s.dir, "mod")
	events, err := testkit.RunCLI(t.Context(), mod, "-json", "./...")
	gotest.NoError(t, err)
	file := filepath.Join(s.dir, "events.json")
	gotest.NoError(t, os.WriteFile(file, []byte(events.Stdout), 0o600))
	s.stream = scenarios(t, mod, "--input="+file, "--format=json")
	s.input = readDoc(t, &s.stream)
	s.verify = scenarios(t, mod, "--verify", "--format=json")
	s.verified = readDoc(t, &s.verify)
}

func (s *ScenariosEvidenceTestSuite) AfterAll(t *gotest.T) {
	if s.dir != "" {
		gotest.NoError(t, os.RemoveAll(s.dir))
	}
}

func (s *ScenariosEvidenceTestSuite) TestEvidence(t *gotest.T) {
	t.When("scenarios is given a saved stream with --input", func(w *gotest.T) {
		w.It("lists each bound scenario as verified, failing, not-run or vacuous as its Its ran", func(it *gotest.T) {
			gotest.Equal(it, "input", s.input.Evidence)
			gotest.Equal(it, map[string]string{
				"A paid order":           "verified",
				"A failed payment":       "failing",
				"A gateway that is down": "not-run",
				"A bank that is offline": "not-run",
				"A receipt":              "vacuous",
				"Parcels":                "failing",
			}, statuses(&s.input, checkoutSpec))
			gotest.Equal(it, "verified", byTitle(it, &s.input, cartSpec, "An empty cart").Status)
		})
		w.It("names the stream's run, its version and arguments and the packages it ran, as the run judged", func(it *gotest.T) {
			gotest.NotZero(it, s.input.Run)
			gotest.NotEmpty(it, s.input.Run.Version)
			gotest.Equal(it, []string{"-json", "./..."}, s.input.Run.Args)
			gotest.Equal(it, []string{"scenmod/checkout", "scenmod/other", "scenmod/shop"}, s.input.Run.Packages)
		})
	})

	t.When("a clause's It was skipped or its suite's guard kept it from running", func(w *gotest.T) {
		w.It("lists the scenario as not-run, not verified", func(it *gotest.T) {
			for _, title := range []string{"A gateway that is down", "A bank that is offline"} {
				sc := byTitle(it, &s.input, checkoutSpec, title)
				gotest.Equal(it, "not-run", sc.Status, title)
				gotest.Equal(it, "not-run", sc.Clauses[0].Status, title)
			}
		})
	})

	t.When("scenarios is given --verify", func(w *gotest.T) {
		w.It("runs the packages that hold a bound It and no other, and judges the scenarios by that run", func(it *gotest.T) {
			gotest.Equal(it, "run", s.verified.Evidence)
			gotest.NotZero(it, s.verified.Run)
			gotest.Equal(it, []string{"scenmod/checkout", "scenmod/shop"}, s.verified.Run.Packages)
			gotest.Equal(it, statuses(&s.input, checkoutSpec), statuses(&s.verified, checkoutSpec))
			gotest.Equal(it, "failing", byTitle(it, &s.verified, checkoutSpec, "A failed payment").Status)
		})
	})
}

func (s *ScenariosEvidenceTestSuite) TestRows(t *gotest.T) {
	t.When("a bound When sits under the rows of an Each table", func(w *gotest.T) {
		w.It("binds each clause to one It, every row's run counting toward its verdict", func(it *gotest.T) {
			sizes := byTitle(it, &s.input, cartSpec, "Sizes")
			gotest.Equal(it, "verified", sizes.Status)
			gotest.Equal(it, []string{"shop/cart_suite_test.go:66"}, at(sizes.Clauses[0].Its))
			parcels := byTitle(it, &s.input, checkoutSpec, "Parcels")
			gotest.Equal(it, "failing", parcels.Status)
			gotest.Equal(it, []string{"checkout/checkout_suite_test.go:41"}, at(parcels.Clauses[0].Its))
		})
	})
}
