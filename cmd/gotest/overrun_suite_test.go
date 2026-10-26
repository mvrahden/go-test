package main_test

import (
	"encoding/json"
	"strings"

	"github.com/mvrahden/go-test/pkg/gotest"
	"github.com/mvrahden/go-test/tests/gotestcli"
)

// OverrunTestSuite runs parallel methods through the built CLI, one of which
// overruns its Timeout: the overrun is reported once, for that method alone.
//
//nolint:lifecycle-pair // BeforeAll only wraps the shared binary, which its fixture removes
type OverrunTestSuite struct {
	CLI *gotestcli.BinarySharedFixture
	cli cliRunner
}

func (s *OverrunTestSuite) SuiteConfig() gotest.SuiteConfig {
	return gotest.SuiteConfig{Parallel: true}
}

func (s *OverrunTestSuite) BeforeAll(t *gotest.T) {
	s.cli = newCLIRunner(s.CLI)
}

// racingRun runs the suite whose methods finish in time while a sibling
// overruns its Timeout.
func (s *OverrunTestSuite) racingRun(t *gotest.T, args ...string) (stdout, stderr string) {
	stdout, stderr, code := s.cli.runSplit(t, append(args, "./cmd/gotest/testdata/overrun/")...)
	gotest.Equal(t, 1, code, "stderr:\n%s", stderr)
	return stdout, stderr
}

// stillRunning lists the lines of out that report a method still running.
func stillRunning(out string) []string {
	var lines []string
	for line := range strings.Lines(out) {
		if strings.Contains(line, "is still running") && !strings.Contains(line, "has been building for") {
			lines = append(lines, strings.TrimSpace(line))
		}
	}
	return lines
}

func (s *OverrunTestSuite) TestTextRace(t *gotest.T) {
	stdout, stderr := s.racingRun(t)

	t.It("reports only the overrunning method as still running", func(it *gotest.T) {
		gotest.Equal(it, []string{"overrun_suite_test.go:21: exceeded its configured Timeout of 100ms and is still running"}, stillRunning(stdout))
	})
	t.It("writes only its overrun unbuffered to stderr", func(it *gotest.T) {
		lines := stillRunning(stderr)
		gotest.Len(it, lines, 1, stderr)
		gotest.Regexp(it, `^gotest: overrun_suite_test\.go:21: TestRacingTestSuite/TestOverruns exceeded`, lines[0])
	})
}

func (s *OverrunTestSuite) TestJSONRace(t *gotest.T) {
	stdout, _ := s.racingRun(t, "-json")

	var running []goTestEvent
	for line := range strings.Lines(stdout) {
		var ev goTestEvent
		gotest.NoError(t, json.Unmarshal([]byte(line), &ev), line)
		if strings.Contains(ev.Output, "is still running") {
			running = append(running, ev)
		}
	}
	t.It("streams one overrun, attributed to the overrunning method", func(it *gotest.T) {
		gotest.Len(it, running, 1, stdout)
		gotest.Equal(it, "TestRacingTestSuite/TestOverruns", running[0].Test)
		gotest.Equal(it, "    overrun_suite_test.go:21: exceeded its configured Timeout of 100ms and is still running\n", running[0].Output)
	})
}
