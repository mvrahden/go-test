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
	cfg := gotest.IntegrationSuiteConfig()
	cfg.Parallel = true
	return cfg
}

func (s *RunFailureStreamTestSuite) BeforeAll(t *gotest.T) {
	s.cli = newCLIRunner(s.CLI)
}

type streamEvent struct {
	Action  string `json:"Action"`
	Package string `json:"Package"`
	Test    string `json:"Test"`
	Output  string `json:"Output"`
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

// A shared fixture that fails to come up fails the run the same way in every
// mode that runs tests: exit 1, and what the mode writes on stdout names the
// suite that could not run, the fixture's own reason, and the verdict of a
// suite the fixture was none of its business.
func (s *RunFailureStreamTestSuite) TestSharedFixtureSetupFailure(t *gotest.T) {
	const (
		pkg      = "./tests/canary/testdata/setupfailing/"
		gaveUp   = "TestSetupFailingTestSuite never ran: shared fixture DownSharedFixture did not come up"
		itsCause = "DownSharedFixture.BeforeAll failed after 1 attempt(s): the fixture would not come up"
	)

	t.When("the run streams events", func(w *gotest.T) {
		stdout, _, code := s.cli.runSplit(w, "-json", pkg)
		var events []streamEvent
		sc := bufio.NewScanner(strings.NewReader(stdout))
		for sc.Scan() {
			var ev streamEvent
			gotest.NoError(w, json.Unmarshal([]byte(sc.Text()), &ev), sc.Text())
			events = append(events, ev)
		}
		verdict := func(pkg, test string) string {
			last := ""
			for _, ev := range events {
				if strings.HasSuffix(ev.Package, pkg) && ev.Test == test && (ev.Action == "pass" || ev.Action == "fail") {
					last = ev.Action
				}
			}
			return last
		}
		output := func(pkg, test string) string {
			var b strings.Builder
			for _, ev := range events {
				if strings.HasSuffix(ev.Package, pkg) && ev.Test == test && ev.Action == "output" {
					b.WriteString(ev.Output)
				}
			}
			return b.String()
		}

		w.It("exits 1", func(it *gotest.T) {
			gotest.Equal(it, 1, code)
		})
		w.It("fails the suite that reads the fixture, saying why", func(it *gotest.T) {
			gotest.Equal(it, "fail", verdict("/setupfailing", "TestSetupFailingTestSuite"))
			gotest.Contains(it, output("/setupfailing", "TestSetupFailingTestSuite"), gaveUp)
		})
		w.It("runs the suite that reads no fixture", func(it *gotest.T) {
			gotest.Equal(it, "pass", verdict("/setupfailing", "TestIndependentTestSuite/TestRuns"))
		})
		w.It("runs the suite that reads a fixture of its own, after the failure", func(it *gotest.T) {
			gotest.Equal(it, "pass", verdict("/setupfailing", "TestAloneTestSuite/TestFixtureIsUp"))
		})
		w.It("books the failure with the fixture's own reason", func(it *gotest.T) {
			gotest.Equal(it, "fail", verdict("shared fixtures", ""))
			gotest.Contains(it, output("shared fixtures", ""), itsCause)
		})
	})

	for t, tc := range gotest.Each(t, []struct {
		Desc string
		args []string
	}{
		{"spec", []string{"spec", "--no-color", pkg}},
		{"spec as markdown", []string{"spec", "--format=md", pkg}},
		{"summary", []string{"summary", "--no-color", pkg}},
	}) {
		stdout, _, code := s.cli.runSplit(t, tc.args...)
		gotest.Equal(t, 1, code, "stdout:\n%s", stdout)
		gotest.Contains(t, stdout, gaveUp)
		gotest.Contains(t, stdout, itsCause)
	}

	t.When("the run prints text", func(w *gotest.T) {
		stdout, stderr, code := s.cli.runSplit(w, pkg)

		w.It("exits 1 and names both reasons on stderr", func(it *gotest.T) {
			gotest.Equal(it, 1, code)
			gotest.Contains(it, stderr, gaveUp)
			gotest.Contains(it, stderr, itsCause)
			gotest.Contains(it, stderr, "FAIL: shared fixture setup failed")
		})
		w.It("does not call the run empty", func(it *gotest.T) {
			gotest.NotContains(it, stdout+stderr, "no test suites to run")
		})
	})
}
