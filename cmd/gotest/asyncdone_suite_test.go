package main_test

import (
	"strconv"

	"github.com/mvrahden/go-test/pkg/gotest"
	"github.com/mvrahden/go-test/tests/gotestcli"
)

// AsyncDoneTestSuite runs an async method that never calls done through the
// built CLI: the verdict names the method's declaration, a file the user has.
//
//nolint:lifecycle-pair // BeforeAll only wraps the shared binary, which its fixture removes
type AsyncDoneTestSuite struct {
	CLI *gotestcli.BinarySharedFixture
	cli cliRunner
}

func (s *AsyncDoneTestSuite) SuiteConfig() gotest.SuiteConfig {
	cfg := gotest.IntegrationSuiteConfig()
	cfg.Parallel = true
	return cfg
}

func (s *AsyncDoneTestSuite) BeforeAll(t *gotest.T) {
	s.cli = newCLIRunner(s.CLI)
}

// asyncVerdict is the failure at the method's declaration, line 16 of the fixture.
const asyncVerdict = "    asyncdone_suite_test.go:16: TestNeverDoneAsync: done() was not called before the test deadline\n"

func (s *AsyncDoneTestSuite) TestTextRun(t *gotest.T) {
	stdout, stderr, code := s.cli.runSplit(t, "./cmd/gotest/testdata/asyncdone/")
	gotest.Equal(t, 1, code, "stderr:\n%s", stderr)

	t.It("locates the verdict at the method's declaration", func(it *gotest.T) {
		gotest.Contains(it, stdout, asyncVerdict)
	})
	t.It("names no file inside gotest", func(it *gotest.T) {
		gotest.NotContains(it, stdout, "methods.go:")
	})
}

func (s *AsyncDoneTestSuite) TestJSONRun(t *gotest.T) {
	stdout, stderr, code := s.cli.runSplit(t, "-json", "./cmd/gotest/testdata/asyncdone/")
	gotest.Equal(t, 1, code, "stderr:\n%s", stderr)

	t.It("streams the verdict at the method's declaration", func(it *gotest.T) {
		gotest.Contains(it, stdout, `"Test":"TestNeverDoneTestSuite/TestNeverDoneAsync","Output":`+strconv.Quote(asyncVerdict))
	})
	t.It("names no file inside gotest", func(it *gotest.T) {
		gotest.NotContains(it, stdout, "methods.go:")
	})
}
