package gotestrunner_test

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/mvrahden/go-test/internal/gotestrunner"
	"github.com/mvrahden/go-test/pkg/gotest"
	"github.com/mvrahden/go-test/pkg/gotestruntime"
)

// A run of this test binary with envStopFileChild set is the suite process
// of the scenario below: a suite whose first method holds until its context
// ends, with two methods after it.
const (
	envStopFileChild = "GOTESTRUNNER_STOPFILE_CHILD"
	envStopFileDir   = "GOTESTRUNNER_STOPFILE_DIR"
	stopFileSuite    = "TestStopFileTestSuite"
	stopFileMethod   = "TestAStopByTheStopFileKeepsTheStream"
)

func stopFileChildRun(t *testing.T) {
	dir := os.Getenv(envStopFileDir)
	t.Run("suite", func(tt *testing.T) { //nolint:suite-lifecycle // the scenario's own suite frame
		cfg, budget := gotestruntime.OpenSuite(tt, gotestruntime.Suite{
			BeforeAll: func(*gotest.T) {},
			AfterAll:  func(*gotest.T) {},
		})
		method := func(name string, body func(*gotest.T)) gotestruntime.Method {
			return gotestruntime.Method{Name: name, Run: func(ttt *gotest.T, run func(func())) {
				run(func() { body(ttt) })
			}}
		}
		gotestruntime.RunMethods(tt, cfg, budget, gotestruntime.Methods{Cases: []gotestruntime.Method{
			method("TestHolds", func(ttt *gotest.T) {
				_ = os.WriteFile(filepath.Join(dir, "running"), nil, 0o600)
				<-ttt.Context().Done()
			}),
			method("TestSecond", func(*gotest.T) {}),
			method("TestThird", func(*gotest.T) {}),
		}})
	})
}

// StopFileTestSuite covers a run stopped by the stop file alone, the way a
// Windows run is: no console event reaches the converter, so the stream
// carries the suite's verdicts to their end.
// Sequential: the scenario swaps how the runner signals a suite.
type StopFileTestSuite struct{}

func (s *StopFileTestSuite) TestAStopByTheStopFileKeepsTheStream(t *gotest.T) {
	if os.Getenv(envStopFileChild) != "" {
		stopFileChildRun(t.T())
		return
	}
	restore := gotestrunner.ExportUseStopFileSignal()
	defer restore()
	dir := t.TempDir()
	budget := filepath.Join(dir, "budget")
	gotest.NoError(t, os.WriteFile(budget, []byte("30s"), 0o600))
	target := gotestrunner.SuiteTarget{
		SuiteSpec: gotestrunner.SuiteSpec{
			Package:   "example.com/stop",
			SuiteName: stopFileSuite,
			RunFilter: "^" + stopFileSuite + "$/^" + stopFileMethod + "$",
		},
		BinaryPath: os.Args[0],
		StopFile:   filepath.Join(dir, "stop"),
		BudgetFile: budget,
	}
	env := append(os.Environ(), envStopFileChild+"=1", envStopFileDir+"="+dir)
	run, cancel := context.WithCancel(context.Background())
	defer cancel()
	go func() {
		for run.Err() == nil {
			if _, err := os.Stat(filepath.Join(dir, "running")); err == nil {
				cancel()
				return
			}
			time.Sleep(20 * time.Millisecond)
		}
	}()
	started := time.Now()
	result := gotestrunner.RunSingleSuite(run, target, env, true)
	took := time.Since(started)

	verdict, output := map[string]string{}, map[string]string{}
	sc := bufio.NewScanner(bytes.NewReader(result.Stdout))
	for sc.Scan() {
		var ev struct{ Action, Test, Output string }
		if json.Unmarshal(sc.Bytes(), &ev) != nil {
			continue
		}
		switch ev.Action {
		case "pass", "fail", "skip":
			verdict[ev.Test] = ev.Action
		case "output":
			output[ev.Test] += ev.Output
		}
	}
	prefix := stopFileSuite + "/" + stopFileMethod + "/suite/"

	t.When("a run is cancelled while a method holds and only the stop file announces it", func(w *gotest.T) {
		w.It("skips the methods not started with the reason run stopping", func(it *gotest.T) {
			for _, m := range []string{"TestSecond", "TestThird"} {
				gotest.Equal(it, "skip", verdict[prefix+m], string(result.Stdout))
				gotest.Contains(it, output[prefix+m], "run stopping")
			}
		})
		w.It("reports the holding method and the suite", func(it *gotest.T) {
			gotest.NotEmpty(it, verdict[prefix+"TestHolds"], string(result.Stdout))
			gotest.NotEmpty(it, verdict[stopFileSuite], string(result.Stdout))
		})
		w.It("leaves the converter alive", func(it *gotest.T) {
			_, how := gotestrunner.ExitStatusVerdict(result.ExitCode)
			gotest.Empty(it, how, string(result.Stderr))
		})
		w.It("ends well within the teardown budget", func(it *gotest.T) {
			gotest.True(it, result.CutShort)
			gotest.Less(it, took, 10*time.Second)
		})
	})
}
