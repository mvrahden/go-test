package cli_test

import (
	"encoding/json"
	"strings"
	"time"

	"github.com/mvrahden/go-test/internal/testkit"
	"github.com/mvrahden/go-test/pkg/gotest"
)

// CLIVersionTestSuite: the one invocation every other check assumes works.
type CLIVersionTestSuite struct{}

func (s *CLIVersionTestSuite) SuiteConfig() gotest.SuiteConfig {
	return gotest.SuiteConfig{Parallel: true, Timeout: 30 * time.Second}
}

func (s *CLIVersionTestSuite) TestVersion(t *gotest.T) {
	run, err := testkit.RunCLI(t.Context(), ".", "version")
	gotest.NoError(t, err)
	t.When("the version subcommand runs", func(w *gotest.T) {
		w.It("exits 0 and prints the version on stdout", func(it *gotest.T) {
			gotest.Equal(it, 0, run.Exit)
			gotest.Regexp(it, "gotest", run.Stdout)
			gotest.Empty(it, run.Stderr)
		})
	})
}

func (s *CLIVersionTestSuite) TestVersionAsJSON(t *gotest.T) {
	t.When("the version subcommand runs with --json", func(w *gotest.T) {
		w.It("prints one JSON document naming the version, the subcommands and the audit's schema", func(it *gotest.T) {
			run, err := testkit.RunCLI(it.Context(), ".", "version", "--json")
			gotest.NoError(it, err)
			gotest.Equal(it, 0, run.Exit)
			gotest.Empty(it, run.Stderr)
			gotest.Equal(it, 1, strings.Count(run.Stdout, "\n"))
			var doc struct {
				Version  string         `json:"version"`
				Commands []string       `json:"commands"`
				Schemas  map[string]int `json:"schemas"`
			}
			gotest.NoError(it, json.Unmarshal([]byte(run.Stdout), &doc), run.Stdout)
			gotest.NotEmpty(it, doc.Version)
			gotest.Contains(it, doc.Commands, "audit")
			gotest.Contains(it, doc.Commands, "version")
			gotest.Equal(it, map[string]int{"audit": 1}, doc.Schemas)
		})
	})
}
