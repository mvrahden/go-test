package cli_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"time"

	"github.com/mvrahden/go-test/internal/testkit"
	"github.com/mvrahden/go-test/pkg/gotest"
)

// CLITestResultTestSuite: gotest summary prints a saved run's result as the
// in-toto test-result predicate.
type CLITestResultTestSuite struct{}

func (s *CLITestResultTestSuite) SuiteConfig() gotest.SuiteConfig {
	return gotest.SuiteConfig{Parallel: true, Timeout: 30 * time.Second}
}

// Streams written by hand: a manifest, the verdicts and the closing result.
const (
	runOpens  = `{"Action":"start","Package":"gotest run","RunLevel":true,"Manifest":{"Version":"v1.40.0","GoVersion":"go1.26.2","Args":["summary","--events-file=run.json","./..."],"Commit":"abc1234","Dirty":false}}`
	failedRun = runOpens + `
{"Action":"run","Package":"example.com/m/calc","Test":"TestCalcTestSuite"}
{"Action":"run","Package":"example.com/m/calc","Test":"TestCalcTestSuite/TestAdds"}
{"Action":"attr","Package":"example.com/m/calc","Test":"TestCalcTestSuite/TestAdds","Key":"gotest.assertions","Value":"2"}
{"Action":"pass","Package":"example.com/m/calc","Test":"TestCalcTestSuite/TestAdds"}
{"Action":"run","Package":"example.com/m/calc","Test":"TestCalcTestSuite/TestDivides"}
{"Action":"attr","Package":"example.com/m/calc","Test":"TestCalcTestSuite/TestDivides","Key":"gotest.assertions","Value":"1"}
{"Action":"fail","Package":"example.com/m/calc","Test":"TestCalcTestSuite/TestDivides"}
{"Action":"run","Package":"example.com/m/calc","Test":"TestCalcTestSuite/TestPrints"}
{"Action":"attr","Package":"example.com/m/calc","Test":"TestCalcTestSuite/TestPrints","Key":"gotest.assertions","Value":"0"}
{"Action":"pass","Package":"example.com/m/calc","Test":"TestCalcTestSuite/TestPrints"}
{"Action":"fail","Package":"example.com/m/calc","Test":"TestCalcTestSuite"}
{"Action":"fail","Package":"example.com/m/calc"}
{"Action":"fail","Package":"gotest run","RunLevel":true,"Result":{"ExitCode":1,"Packages":{"Passed":0,"Failed":1,"Skipped":0},"Tests":{"Passed":2,"Failed":2,"Skipped":0}}}
`
	passedRun = runOpens + `
{"Action":"run","Package":"example.com/m/calc","Test":"TestCalcTestSuite"}
{"Action":"run","Package":"example.com/m/calc","Test":"TestCalcTestSuite/TestAdds"}
{"Action":"attr","Package":"example.com/m/calc","Test":"TestCalcTestSuite/TestAdds","Key":"gotest.assertions","Value":"2"}
{"Action":"pass","Package":"example.com/m/calc","Test":"TestCalcTestSuite/TestAdds"}
{"Action":"pass","Package":"example.com/m/calc","Test":"TestCalcTestSuite"}
{"Action":"pass","Package":"example.com/m/calc"}
{"Action":"pass","Package":"gotest run","RunLevel":true,"Result":{"ExitCode":0,"Packages":{"Passed":1,"Failed":0,"Skipped":0},"Tests":{"Passed":2,"Failed":0,"Skipped":0}}}
`
	plainStream = `{"Action":"run","Package":"example.com/m/calc","Test":"TestAdds"}
{"Action":"pass","Package":"example.com/m/calc","Test":"TestAdds"}
{"Action":"pass","Package":"example.com/m/calc"}
`
)

// predicate is the part of the in-toto test-result predicate the suite reads.
type predicate struct {
	Result        string `json:"result"`
	Configuration []struct {
		Name        string `json:"name"`
		URI         string `json:"uri"`
		Annotations struct {
			Version   string   `json:"version"`
			GoVersion string   `json:"goVersion"`
			Args      []string `json:"args"`
		} `json:"annotations"`
	} `json:"configuration"`
	URL         string   `json:"url"`
	PassedTests []string `json:"passedTests"`
	WarnedTests []string `json:"warnedTests"`
	FailedTests []string `json:"failedTests"`
}

// testResult writes stream to a file and runs summary over it.
func testResult(t *gotest.T, stream string, env ...string) testkit.Run {
	dir := t.TempDir()
	file := filepath.Join(dir, "run.json")
	gotest.NoError(t, os.WriteFile(file, []byte(stream), 0o600))
	run, err := testkit.RunCLIWithEnv(t.Context(), dir, env, "summary", "--input="+file, "--format=intoto-test-result")
	gotest.NoError(t, err)
	return run
}

func readPredicate(t *gotest.T, run *testkit.Run) predicate {
	gotest.Equal(t, 0, run.Exit, "stderr: %s", run.Stderr)
	gotest.Empty(t, run.Stderr)
	var p predicate
	gotest.NoError(t, json.Unmarshal([]byte(run.Stdout), &p), "stdout: %s", run.Stdout)
	return p
}

func (s *CLITestResultTestSuite) TestThePredicate(t *gotest.T) {
	t.When("summary writes the test-result predicate of a stream whose run failed a test and passed one without an assertion", func(w *gotest.T) {
		run := testResult(w, failedRun)
		w.It("prints result FAILED with the failed test in failedTests, the one without an assertion in warnedTests and the others in passedTests", func(it *gotest.T) {
			p := readPredicate(it, &run)
			gotest.Equal(it, "FAILED", p.Result)
			gotest.Equal(it, []string{"example.com/m/calc.TestCalcTestSuite/TestDivides"}, p.FailedTests)
			gotest.Equal(it, []string{"example.com/m/calc.TestCalcTestSuite/TestPrints"}, p.WarnedTests)
			gotest.Equal(it, []string{"example.com/m/calc.TestCalcTestSuite/TestAdds"}, p.PassedTests)
			gotest.Empty(it, p.URL)
		})
		w.It("names the CLI version, the Go version and the arguments the manifest records in its configuration", func(it *gotest.T) {
			p := readPredicate(it, &run)
			gotest.Len(it, p.Configuration, 1)
			c := p.Configuration[0]
			gotest.Equal(it, "gotest", c.Name)
			gotest.Equal(it, "pkg:golang/github.com/mvrahden/go-test@v1.40.0", c.URI)
			gotest.Equal(it, "v1.40.0", c.Annotations.Version)
			gotest.Equal(it, "go1.26.2", c.Annotations.GoVersion)
			gotest.Equal(it, []string{"summary", "--events-file=run.json", "./..."}, c.Annotations.Args)
		})
	})
	t.When("summary writes the test-result predicate of a stream whose run passed, under GitHub Actions", func(w *gotest.T) {
		run := testResult(w, passedRun, "GITHUB_ACTIONS=true", "GITHUB_SERVER_URL=https://github.example", "GITHUB_REPOSITORY=o/r", "GITHUB_RUN_ID=42", "GITHUB_RUN_ATTEMPT=3")
		w.It("prints result PASSED and the url of the Actions run", func(it *gotest.T) {
			p := readPredicate(it, &run)
			gotest.Equal(it, "PASSED", p.Result)
			gotest.Equal(it, "https://github.example/o/r/actions/runs/42/attempts/3", p.URL)
			gotest.Equal(it, []string{"example.com/m/calc.TestCalcTestSuite/TestAdds"}, p.PassedTests)
			gotest.Empty(it, p.FailedTests)
		})
	})
	t.When("summary writes the test-result predicate of a stream without a run manifest", func(w *gotest.T) {
		run := testResult(w, plainStream)
		w.It("exits 2 naming --events-file on stderr, with nothing on stdout", func(it *gotest.T) {
			gotest.Equal(it, 2, run.Exit)
			gotest.Empty(it, run.Stdout)
			gotest.Regexp(it, "^FAIL: ", run.Stderr)
			gotest.Contains(it, run.Stderr, "--events-file")
		})
	})
}
