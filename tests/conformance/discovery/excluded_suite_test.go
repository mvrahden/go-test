package discovery_test

import (
	"encoding/json"
	"strings"
	"time"

	"github.com/mvrahden/go-test/internal/testkit"
	"github.com/mvrahden/go-test/pkg/gotest"
)

// ExcludedAndDeclaredTestSuite: an X_ suite is reported and never run, and
// discover says which call declared each behavior.
type ExcludedAndDeclaredTestSuite struct{}

func (s *ExcludedAndDeclaredTestSuite) SuiteConfig() gotest.SuiteConfig {
	return gotest.SuiteConfig{Parallel: true, Timeout: 30 * time.Second}
}

func (s *ExcludedAndDeclaredTestSuite) TestAnExcludedSuite(t *gotest.T) {
	run, err := testkit.RunPackage(t.Context(), t.TempDir(), "../testdata/e2eexcluded/")
	gotest.NoError(t, err)
	t.When("a package holds an X_ suite beside a suite that runs", func(w *gotest.T) {
		w.It("reports the X_ suite skipped as excluded by the user, runs none of its methods and passes the suite beside it", func(it *gotest.T) {
			gotest.Equal(it, 0, run.Exit, "stdout: %s\nstderr: %s", run.Stdout, run.Stderr)
			gotest.Equal(it, "pass", run.Verdict("TestActiveTestSuite/TestRuns"), "stdout: %s", run.Stdout)
			gotest.Equal(it, "skip", run.Verdict("TestX_RetiredTestSuite"), "stdout: %s", run.Stdout)
			var said bool
			for _, ev := range run.Events {
				gotest.NotEqual(it, "TestX_RetiredTestSuite/TestRetired", ev.Test, "an excluded method ran")
				said = said || ev.Test == "TestX_RetiredTestSuite" && strings.Contains(ev.Output, "excluded by user")
			}
			gotest.True(it, said, "no event says the suite was excluded:\n%s", run.Stdout)
		})
	})
}

type declaredBehavior struct {
	Name     string             `json:"name"`
	Display  string             `json:"display"`
	Kind     string             `json:"kind"`
	Children []declaredBehavior `json:"children"`
}

func (s *ExcludedAndDeclaredTestSuite) TestBehaviorKinds(t *gotest.T) {
	run, err := testkit.RunCLI(t.Context(), ".", "discover", "../testdata/e2elabels/")
	gotest.NoError(t, err)
	var doc struct {
		Packages []struct {
			Suites []struct {
				Methods []struct {
					Behaviors []declaredBehavior `json:"behaviors"`
				} `json:"methods"`
			} `json:"suites"`
		} `json:"packages"`
	}
	gotest.NoError(t, json.Unmarshal([]byte(run.Stdout), &doc), "discover: %s", run.Stdout)
	kinds := map[string]string{}
	var walk func([]declaredBehavior)
	walk = func(bs []declaredBehavior) {
		for _, b := range bs {
			kinds[b.Display] = b.Kind
			walk(b.Children)
		}
	}
	for _, p := range doc.Packages {
		for _, su := range p.Suites {
			for _, m := range su.Methods {
				walk(m.Behaviors)
			}
		}
	}
	t.When("a method declares a When with Its under it", func(w *gotest.T) {
		w.It("discover reports the When's kind as when and each It's as it", func(it *gotest.T) {
			gotest.Equal(it, map[string]string{
				"when the payload uses snake_case": "when",
				"returns snake_case keys":          "it",
				"leaves CamelCase alone":           "it",
				"with an empty payload":            "when",
				"returns no keys":                  "it",
			}, kinds, "discover: %s", run.Stdout)
		})
	})
}
