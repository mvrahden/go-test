package gotestrunner_test

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/mvrahden/go-test/internal/gotestrunner"
	"github.com/mvrahden/go-test/pkg/gotest"
	"github.com/mvrahden/go-test/pkg/gotestruntime"
)

// A run of this test binary with envChannelMode set is the suite process of a
// scenario below: a suite whose first method acts by the mode, with two
// methods after it.
const (
	envChannelMode = "GOTESTRUNNER_CHANNEL_MODE"
	envChannelDir  = "GOTESTRUNNER_CHANNEL_DIR"
	channelSuite   = "TestChannelStopTestSuite"
)

// What the first method does.
const (
	channelHold  = "hold"  // waits for its context to end
	channelSleep = "sleep" // sleeps a second and passes
	channelTerm  = "term"  // sends its own process SIGTERM, then waits for its context
)

func channelMark(m string) {
	f, err := os.OpenFile(filepath.Join(os.Getenv(envChannelDir), "markers"), os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o600) //nolint:gosec // G304: the scenario's own dir
	if err != nil {
		return
	}
	_, _ = f.WriteString(m + "\n")
	_ = f.Close()
}

func channelChildRun(t *testing.T) {
	mode := os.Getenv(envChannelMode)
	sigs := make(chan os.Signal, 4)
	signal.Notify(sigs, os.Interrupt, syscall.SIGTERM)
	defer signal.Stop(sigs)
	t.Run("suite", func(tt *testing.T) { //nolint:suite-lifecycle // the scenario's own suite frame
		cfg, budget := gotestruntime.OpenSuite(tt, gotestruntime.Suite{
			BeforeAll: func(*gotest.T) {},
			AfterAll: func(*gotest.T) {
				select {
				case sig := <-sigs:
					channelMark("signal " + sig.String())
				default:
				}
				channelMark("AfterAll")
			},
		})
		method := func(name string, body func(*gotest.T)) gotestruntime.Method {
			return gotestruntime.Method{Name: name, Run: func(ttt *gotest.T, run func(func())) {
				run(func() { body(ttt) })
			}}
		}
		gotestruntime.RunMethods(tt, cfg, budget, gotestruntime.Methods{Cases: []gotestruntime.Method{
			method("TestFirst", func(ttt *gotest.T) {
				switch mode {
				case channelSleep:
					time.Sleep(time.Second)
					return
				case channelTerm:
					if self, err := os.FindProcess(os.Getpid()); err == nil {
						_ = self.Signal(syscall.SIGTERM) // no such signal on Windows
					}
				}
				channelMark("running")
				<-ttt.Context().Done()
			}),
			method("TestSecond", func(*gotest.T) {}),
			method("TestThird", func(*gotest.T) {}),
		}})
	})
}

// channelRun is one scenario's suite process run by the runner.
type channelRun struct {
	dir    string
	result gotestrunner.SuiteResult
	took   time.Duration
}

// runChannelSuite runs method's suite process with mode, under exec when
// given, on the JSON pipeline; cancel says whether the run is cancelled once
// the first method runs.
func runChannelSuite(t *gotest.T, method, mode string, exec []string, cancel bool) *channelRun {
	r := &channelRun{dir: t.TempDir()}
	target := gotestrunner.SuiteTarget{
		SuiteSpec: gotestrunner.SuiteSpec{
			Package:   "example.com/stop",
			SuiteName: channelSuite,
			RunFilter: "^" + channelSuite + "$/^" + method + "$",
		},
		BinaryPath: os.Args[0],
		Exec:       exec,
	}
	env := append(os.Environ(), envChannelMode+"="+mode, envChannelDir+"="+r.dir)
	run, stop := context.WithCancel(context.Background())
	defer stop()
	if cancel {
		go func() {
			for run.Err() == nil {
				if strings.Contains(r.markers(), "running") {
					stop()
					return
				}
				time.Sleep(20 * time.Millisecond)
			}
		}()
	}
	started := time.Now()
	r.result = gotestrunner.RunSingleSuite(run, target, env, true)
	r.took = time.Since(started)
	return r
}

func (r *channelRun) markers() string {
	b, _ := os.ReadFile(filepath.Join(r.dir, "markers")) //nolint:gosec // G304: the scenario's own dir
	return string(b)
}

// verdicts maps each test of the run's stream to its verdict, and to its
// output.
func (r *channelRun) verdicts(method string) (verdict, output map[string]string) {
	verdict, output = map[string]string{}, map[string]string{}
	sc := bufio.NewScanner(bytes.NewReader(r.result.Stdout))
	for sc.Scan() {
		var ev struct{ Action, Test, Output string }
		if json.Unmarshal(sc.Bytes(), &ev) != nil {
			continue
		}
		name := strings.TrimPrefix(ev.Test, channelSuite+"/"+method+"/suite/")
		switch ev.Action {
		case "pass", "fail", "skip":
			verdict[name] = ev.Action
		case "output":
			output[name] += ev.Output
		}
	}
	return verdict, output
}

// ChannelStopTestSuite covers how the runner stops a suite process that took
// its channel: by the stop line, never by a signal.
type ChannelStopTestSuite struct{}

// SuiteConfig: a method took at most 2s locally; the default is far above
// twice that.
func (s *ChannelStopTestSuite) SuiteConfig() gotest.SuiteConfig {
	cfg := gotest.DefaultSuiteConfig()
	cfg.Parallel = true
	return cfg
}

func (s *ChannelStopTestSuite) TestAStopGoesOutAsALine(t *gotest.T) {
	if os.Getenv(envChannelMode) != "" {
		channelChildRun(t.T())
		return
	}
	r := runChannelSuite(t, "TestAStopGoesOutAsALine", channelHold, nil, true)
	verdict, output := r.verdicts("TestAStopGoesOutAsALine")

	t.When("a run is cancelled while a method of a suite process on its channel holds", func(w *gotest.T) {
		w.It("skips the methods not started with the reason run stopping", func(it *gotest.T) {
			for _, m := range []string{"TestSecond", "TestThird"} {
				gotest.Equal(it, "skip", verdict[m], string(r.result.Stdout))
				gotest.Contains(it, output[m], "run stopping")
			}
		})
		w.It("sends the suite process no signal, and AfterAll runs", func(it *gotest.T) {
			gotest.Equal(it, "AfterAll\n", strings.TrimPrefix(r.markers(), "running\n"))
		})
		w.It("ends by the suite's own status, cut short and stopped by the runner", func(it *gotest.T) {
			_, how := gotestrunner.ExitStatusVerdict(r.result.ExitCode)
			gotest.Empty(it, how, string(r.result.Stderr))
			gotest.True(it, r.result.CutShort)
			gotest.Empty(it, r.result.StoppedBy)
			gotest.Less(it, r.took, 10*time.Second)
		})
	})
}
