package main_test

import (
	"github.com/mvrahden/go-test/pkg/gotest"
	"github.com/mvrahden/go-test/tests/gotestcli"
)

// SharedStateFilesTestSuite runs two packages whose same-named suites need
// different shared fixtures in one run: each suite process must read its own
// state file.
//
//nolint:lifecycle-pair // BeforeAll only wraps the shared binary, which its fixture removes
type SharedStateFilesTestSuite struct {
	CLI *gotestcli.BinarySharedFixture
	cli cliRunner
}

func (s *SharedStateFilesTestSuite) SuiteConfig() gotest.SuiteConfig {
	return gotest.IntegrationSuiteConfig()
}

func (s *SharedStateFilesTestSuite) BeforeAll(t *gotest.T) {
	s.cli = newCLIRunner(s.CLI)
}

func (s *SharedStateFilesTestSuite) TestSameNamedSuitesInOneRun(t *gotest.T) {
	out, code := s.cli.runExit(t, "./tests/sharedfixture/standalone/", "./tests/sharedfixture/twin/")
	gotest.Equal(t, 0, code, out)
	gotest.Contains(t, out, "ok  \tgithub.com/mvrahden/go-test/tests/sharedfixture/standalone")
	gotest.Contains(t, out, "ok  \tgithub.com/mvrahden/go-test/tests/sharedfixture/twin")
}
