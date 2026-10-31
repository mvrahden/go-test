package discovery_test

import (
	"encoding/json"
	"time"

	"github.com/mvrahden/go-test/internal/testkit"
	"github.com/mvrahden/go-test/pkg/gotest"
)

// ListingTestSuite: discover lists what the source declares, not what a run
// would select.
type ListingTestSuite struct{}

func (s *ListingTestSuite) SuiteConfig() gotest.SuiteConfig {
	return gotest.SuiteConfig{Parallel: true, Timeout: 30 * time.Second}
}

// listedMethod is a method as discover lists it.
type listedMethod struct {
	Name     string `json:"name"`
	Focused  bool   `json:"focused"`
	Excluded bool   `json:"excluded"`
}

func (s *ListingTestSuite) TestDeclaredMethods(t *gotest.T) {
	t.When("a suite declares an excluded method and a focused one", func(w *gotest.T) {
		w.It("discover lists the excluded method marked excluded and the unfocused methods beside the focused one", func(it *gotest.T) {
			run, err := testkit.RunCLI(it.Context(), ".", "discover", "../testdata/shapes/listed/")
			gotest.NoError(it, err)
			gotest.Equal(it, 0, run.Exit, "stdout: %s\nstderr: %s", run.Stdout, run.Stderr)
			var doc struct {
				Packages []struct {
					Suites []struct {
						Methods []listedMethod `json:"methods"`
					} `json:"suites"`
				} `json:"packages"`
			}
			gotest.NoError(it, json.Unmarshal([]byte(run.Stdout), &doc), "stdout: %s", run.Stdout)
			gotest.Len(it, doc.Packages, 1, "stdout: %s", run.Stdout)
			gotest.Len(it, doc.Packages[0].Suites, 1, "stdout: %s", run.Stdout)
			gotest.ElementsMatch(it, []listedMethod{
				{Name: "F_TestFocused", Focused: true},
				{Name: "X_TestParked", Excluded: true},
				{Name: "TestPlain"},
			}, doc.Packages[0].Suites[0].Methods)
		})
	})
}
