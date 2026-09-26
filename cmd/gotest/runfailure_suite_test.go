package main_test

import (
	"bufio"
	"encoding/json"
	"path/filepath"
	"strings"

	"github.com/mvrahden/go-test/pkg/gotest"
	"github.com/mvrahden/go-test/tests/gotestcli"
)

// RunFailureStreamTestSuite drives a run whose only failure lands after the
// last suite verdict, and reads it back from the live -json stream: the one
// place a CI parser or gotestsum ever looks.
//
//nolint:lifecycle-pair // BeforeAll only wraps the shared binary, which its fixture removes
type RunFailureStreamTestSuite struct {
	CLI *gotestcli.BinarySharedFixture
	cli cliRunner
}

func (s *RunFailureStreamTestSuite) SuiteConfig() gotest.SuiteConfig {
	return gotest.IntegrationSuiteConfig()
}

func (s *RunFailureStreamTestSuite) BeforeAll(t *gotest.T) {
	s.cli = newCLIRunner(s.CLI)
}

type streamVerdict struct {
	Action  string `json:"Action"`
	Package string `json:"Package"`
	Test    string `json:"Test"`
}

func (s *RunFailureStreamTestSuite) TestSharedFixtureTeardownFailure(t *gotest.T) {
	dir := filepath.Join(s.cli.repoRoot, "vscode-gotest", "testdata", "teardownfail")
	out, code := runGotestIn(t, s.cli.binary, dir, []string{"GOWORK=off", "GOTEST_TEARDOWN_FAIL=1"}, "-json", "./...")

	var verdicts []streamVerdict
	sc := bufio.NewScanner(strings.NewReader(out))
	for sc.Scan() {
		if !strings.HasPrefix(sc.Text(), "{") {
			continue
		}
		var v streamVerdict
		gotest.NoError(t, json.Unmarshal([]byte(sc.Text()), &v))
		if v.Test == "" && (v.Action == "pass" || v.Action == "fail") {
			verdicts = append(verdicts, v)
		}
	}

	t.It("fails the run", func(it *gotest.T) {
		gotest.Equal(it, 1, code, "output:\n%s", out)
	})

	t.It("books the failure into the stream as a failed package", func(it *gotest.T) {
		gotest.Contains(it, verdicts, streamVerdict{Action: "fail", Package: "shared fixtures"},
			"a -json consumer must see the failure the exit code reports; got %v", verdicts)
	})

	t.It("keeps the suite's own package verdict", func(it *gotest.T) {
		gotest.Contains(it, verdicts, streamVerdict{Action: "pass", Package: "gotest.teardownfail"})
	})
}
