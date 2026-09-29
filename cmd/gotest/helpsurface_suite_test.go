package main_test

import (
	"regexp"

	. "github.com/mvrahden/go-test/cmd/gotest"
	"github.com/mvrahden/go-test/pkg/gotest"
	"github.com/mvrahden/go-test/tests/gotestcli"
)

// HelpSurfaceTestSuite is a drift guard: the help of a subcommand names every
// flag the subcommand accepts.
//
//nolint:lifecycle-pair // BeforeAll only wraps the shared binary, which its fixture removes
type HelpSurfaceTestSuite struct {
	CLI *gotestcli.BinarySharedFixture
	cli cliRunner
}

func (s *HelpSurfaceTestSuite) SuiteConfig() gotest.SuiteConfig {
	cfg := gotest.IntegrationSuiteConfig()
	cfg.Parallel = true
	return cfg
}

func (s *HelpSurfaceTestSuite) BeforeAll(t *gotest.T) {
	s.cli = newCLIRunner(s.CLI)
}

func (s *HelpSurfaceTestSuite) TestHelpNamesTheFlagsASubcommandAccepts(t *gotest.T) {
	for t, tc := range gotest.Each(t, []struct {
		Name    string
		allowed map[string]bool
	}{
		{"test", ExportTestAllowed},
		{"spec", ExportSpecAllowed},
		{"summary", ExportSummaryAllowed},
		{"watch", ExportWatchAllowed},
		{"bench", ExportBenchAllowed},
		{"fuzz", ExportFuzzAllowed},
		{"migrate", ExportMigrateAllowed},
	}) {
		help, _, code := s.cli.runSplit(t, "help", tc.Name)
		gotest.Equal(t, 0, code)
		for flag := range tc.allowed {
			// The flag as a word of its own: --parallel is not named by
			// --compile-parallel.
			named := regexp.MustCompile(`(^|[^\w-])` + regexp.QuoteMeta(flag) + `($|[^\w-])`)
			gotest.Regexp(t, named, help, "gotest help %s does not name %s", tc.Name, flag)
		}
	}
}
