package main_test

import (
	"bufio"
	"bytes"
	"encoding/json"
	"os/exec"
	"strings"
	"time"

	"github.com/mvrahden/go-test/pkg/gotest"
	"github.com/mvrahden/go-test/tests/gotestcli"
)

// JSONStreamTestSuite pins the live -json stream to the event schema of
// go test -json, which is what its consumers parse it as.
//
//nolint:lifecycle-pair // BeforeAll only wraps the shared binary, which its fixture removes
type JSONStreamTestSuite struct {
	CLI *gotestcli.BinarySharedFixture
	cli cliRunner
}

func (s *JSONStreamTestSuite) SuiteConfig() gotest.SuiteConfig {
	cfg := gotest.IntegrationSuiteConfig()
	cfg.Parallel = true
	return cfg
}

func (s *JSONStreamTestSuite) BeforeAll(t *gotest.T) {
	s.cli = newCLIRunner(s.CLI)
}

// goTestEvent is cmd/test2json's TestEvent with the build fields cmd/go adds.
type goTestEvent struct {
	Time        *time.Time `json:",omitempty"`
	Action      string
	Package     string  `json:",omitempty"`
	ImportPath  string  `json:",omitempty"`
	Test        string  `json:",omitempty"`
	Elapsed     float64 `json:",omitempty"`
	Output      string  `json:",omitempty"`
	OutputType  string  `json:",omitempty"`
	FailedBuild string  `json:",omitempty"`
}

var goTestActions = []string{"start", "run", "pause", "cont", "pass", "bench", "fail", "output", "skip", "build-output", "build-fail", "attr"}

func (s *JSONStreamTestSuite) TestEveryLineIsAGoTestEvent(t *gotest.T) {
	for t, tc := range gotest.Each(t, []struct {
		Desc string
		pkg  string
	}{
		{"a green run", "passing"},
		{"failing tests", "failing"},
		{"a package that does not build", "broken"},
		{"a shared fixture that fails to set up", "setupfailing"},
		{"a shared fixture that fails to tear down", "teardownfailing"},
	}) {
		cmd := exec.Command(s.cli.binary, "-json", "./tests/canary/testdata/"+tc.pkg+"/") //nolint:gosec // G204: controlled binary with fixed args
		cmd.Dir = s.cli.repoRoot
		var stdout bytes.Buffer
		cmd.Stdout = &stdout
		_ = cmd.Run()

		lines := 0
		sc := bufio.NewScanner(&stdout)
		sc.Buffer(make([]byte, 0, 64*1024), 1024*1024)
		for sc.Scan() {
			lines++
			dec := json.NewDecoder(strings.NewReader(sc.Text()))
			dec.DisallowUnknownFields()
			var ev goTestEvent
			gotest.NoError(t, dec.Decode(&ev), "line %d: %s", lines, sc.Text())
			gotest.Contains(t, goTestActions, ev.Action, "line %d: %s", lines, sc.Text())
			// A build event is keyed by what was built; every other event
			// by the package it belongs to. go test never writes both.
			if ev.Action == "build-output" || ev.Action == "build-fail" {
				gotest.NotEmpty(t, ev.ImportPath, "line %d names no build: %s", lines, sc.Text())
				gotest.Empty(t, ev.Package, "line %d: a build event carries no package: %s", lines, sc.Text())
			} else {
				gotest.NotEmpty(t, ev.Package, "line %d names no package: %s", lines, sc.Text())
			}
		}
		gotest.NoError(t, sc.Err())
		gotest.NotZero(t, lines, "the run wrote no stream")
	}
}

// TestABuildFailureTakesGoTestsShape: the diagnostics travel as build-output
// events keyed by the build, build-fail closes them, and the package's
// verdict names that build — what gotestsum, the editor and a replay read.
func (s *JSONStreamTestSuite) TestABuildFailureTakesGoTestsShape(t *gotest.T) {
	cmd := exec.Command(s.cli.binary, "-json", "./tests/canary/testdata/broken/") //nolint:gosec // G204: controlled binary with fixed args
	cmd.Dir = s.cli.repoRoot
	var stdout bytes.Buffer
	cmd.Stdout = &stdout
	_ = cmd.Run()

	var builds []goTestEvent
	var verdict goTestEvent
	diagnostics := ""
	for line := range strings.Lines(stdout.String()) {
		var ev goTestEvent
		gotest.NoError(t, json.Unmarshal([]byte(line), &ev))
		switch {
		case ev.Action == "build-output":
			diagnostics += ev.Output
		case ev.Action == "build-fail":
			builds = append(builds, ev)
		case ev.Action == "fail" && ev.Test == "":
			verdict = ev
		case ev.Action == "output" && ev.Test == "":
			gotest.NotContains(t, ev.Output, ":", "a diagnostic left the build events: %s", ev.Output)
		}
	}
	gotest.Len(t, builds, 1)
	gotest.Regexp(t, `^github.com/mvrahden/go-test/tests/canary/testdata/broken(_test)? \[.*\.test\]$`, builds[0].ImportPath)
	gotest.Equal(t, builds[0].ImportPath, verdict.FailedBuild)
	gotest.Equal(t, "github.com/mvrahden/go-test/tests/canary/testdata/broken", verdict.Package)
	gotest.Regexp(t, `broken_suite_test\.go:\d+:\d+: `, diagnostics)
}
