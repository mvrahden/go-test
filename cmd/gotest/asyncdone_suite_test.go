package main_test

import (
	"encoding/json"
	"path/filepath"
	"strconv"
	"strings"

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
	// No deadline: this suite compiles code, so its time is not a verdict.
	return gotest.SuiteConfig{Parallel: true, Timeout: gotest.NoDeadline}
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

func (s *AsyncDoneTestSuite) TestFullPath(t *gotest.T) {
	stdout, stderr, code := s.cli.runSplit(t, "-fullpath", "-run", "^TestNeverDoneTestSuite$", "./cmd/gotest/testdata/asyncdone/")
	gotest.Equal(t, 1, code, "stderr:\n%s", stderr)

	t.It("names the declaration by its full path, as go test -fullpath does", func(it *gotest.T) {
		file := filepath.ToSlash(filepath.Join(s.cli.repoRoot, "cmd", "gotest", "testdata", "asyncdone", "asyncdone_suite_test.go"))
		gotest.Contains(it, stdout, file+":16: TestNeverDoneAsync: done() was not called before the test deadline\n")
	})
}

// racingRun runs the suite whose async methods wait on done while a sibling
// overruns its Timeout.
func (s *AsyncDoneTestSuite) racingRun(t *gotest.T, args ...string) (stdout, stderr string) {
	stdout, stderr, code := s.cli.runSplit(t, append(args, "-run", "^TestRacingTestSuite$", "./cmd/gotest/testdata/asyncdone/")...)
	gotest.Equal(t, 1, code, "stderr:\n%s", stderr)
	return stdout, stderr
}

// stillRunning lists the lines of out that report a method still running.
func stillRunning(out string) []string {
	var lines []string
	for line := range strings.Lines(out) {
		if strings.Contains(line, "is still running") {
			lines = append(lines, strings.TrimSpace(line))
		}
	}
	return lines
}

func (s *AsyncDoneTestSuite) TestTextRace(t *gotest.T) {
	stdout, stderr := s.racingRun(t)

	t.It("fails each method waiting on done at its declaration", func(it *gotest.T) {
		gotest.Contains(it, stdout, "asyncdone_suite_test.go:26: TestFirstNeverDoneAsync: done() was not called before the test deadline\n")
		gotest.Contains(it, stdout, "asyncdone_suite_test.go:28: TestSecondNeverDoneAsync: done() was not called before the test deadline\n")
	})
	t.It("reports only the overrunning method as still running", func(it *gotest.T) {
		gotest.Equal(it, []string{"asyncdone_suite_test.go:30: exceeded its configured Timeout of 100ms and is still running"}, stillRunning(stdout))
	})
	t.It("writes only its overrun unbuffered to stderr", func(it *gotest.T) {
		lines := stillRunning(stderr)
		gotest.Len(it, lines, 1, stderr)
		gotest.Regexp(it, `^gotest: TestRacingTestSuite/TestOverruns exceeded`, lines[0])
	})
}

func (s *AsyncDoneTestSuite) TestJSONRace(t *gotest.T) {
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
		gotest.Equal(it, "    asyncdone_suite_test.go:30: exceeded its configured Timeout of 100ms and is still running\n", running[0].Output)
	})
}
