package cli_test

import (
	"encoding/json"
	"strings"
	"time"

	"github.com/mvrahden/go-test/internal/testkit"
	"github.com/mvrahden/go-test/pkg/gotest"
)

// CLIGroupSummaryTestSuite: the summary names each ExclusiveGroup that ran,
// with its suites and wall time.
type CLIGroupSummaryTestSuite struct{}

func (s *CLIGroupSummaryTestSuite) SuiteConfig() gotest.SuiteConfig {
	// Two runs of about 3s plus their build, twice that cold under -race.
	return gotest.SuiteConfig{Timeout: time.Minute}
}

func (s *CLIGroupSummaryTestSuite) TestTheSummaryNamesEachGroup(t *gotest.T) {
	text, err := testkit.RunCLI(t.Context(), ".", "summary", "--no-color", "../testdata/groups/")
	gotest.NoError(t, err)
	doc, err := testkit.RunCLI(t.Context(), ".", "summary", "--format=json", "../testdata/groups/")
	gotest.NoError(t, err)
	t.When("gotest summary runs suites of named groups", func(w *gotest.T) {
		w.It("prints one line per named group with its suites and wall time", func(it *gotest.T) {
			gotest.Equal(it, 0, text.Exit, "stdout: %s\nstderr: %s", text.Stdout, text.Stderr)
			gotest.Regexp(it, `(?m)^group e2e: 3 suites, [0-9.]+m?s, run apart$`, text.Stdout)
			gotest.Regexp(it, `(?m)^group load: 1 suite, [0-9.]+m?s, run apart$`, text.Stdout)
			gotest.Equal(it, 2, strings.Count(text.Stdout, "run apart"), "stdout: %s", text.Stdout)
		})
		w.It("puts a groups entry with each group's suites and wall time in the JSON summary", func(it *gotest.T) {
			gotest.Equal(it, 0, doc.Exit, "stdout: %s\nstderr: %s", doc.Stdout, doc.Stderr)
			var root struct {
				Groups []struct {
					Name   string  `json:"name"`
					Suites int     `json:"suites"`
					Wall   float64 `json:"wall"`
				} `json:"groups"`
			}
			gotest.NoError(it, json.Unmarshal([]byte(doc.Stdout), &root))
			gotest.Len(it, root.Groups, 2, "stdout: %s", doc.Stdout)
			gotest.Equal(it, "e2e", root.Groups[0].Name)
			gotest.Equal(it, 3, root.Groups[0].Suites)
			gotest.GreaterOrEqual(it, root.Groups[0].Wall, 1.0, "two 1s suites and a 0.2s one")
			gotest.Equal(it, "load", root.Groups[1].Name)
			gotest.Equal(it, 1, root.Groups[1].Suites)
		})
	})
}
