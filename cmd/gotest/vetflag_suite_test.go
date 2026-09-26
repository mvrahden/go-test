package main_test

import (
	"github.com/mvrahden/go-test/pkg/gotest"
	"github.com/mvrahden/go-test/tests/gotestcli"
)

// VetFlagTestSuite runs the built CLI with go test's -vet flag, which has to
// reach `go test -c` and nothing else.
//
//nolint:lifecycle-pair // BeforeAll only wraps the shared binary, which its fixture removes
type VetFlagTestSuite struct {
	CLI *gotestcli.BinarySharedFixture
	cli cliRunner
}

func (s *VetFlagTestSuite) SuiteConfig() gotest.SuiteConfig {
	return gotest.IntegrationSuiteConfig()
}

func (s *VetFlagTestSuite) BeforeAll(t *gotest.T) {
	s.cli = newCLIRunner(s.CLI)
}

func (s *VetFlagTestSuite) TestVetOff(t *gotest.T) {
	t.It("accepts -vet=off and runs the package", func(it *gotest.T) {
		out, code := s.cli.runExit(it, "-vet=off", "./tests/canary/testdata/passing/")
		gotest.Equal(it, 0, code, out)
		gotest.Contains(it, out, "ok  \t")
	})

	t.It("rejects --vet", func(it *gotest.T) {
		out, code := s.cli.runExit(it, "--vet", "./tests/canary/testdata/passing/")
		gotest.Equal(it, 2, code, out)
		gotest.Contains(it, out, "unknown flag: --vet")
	})
}
