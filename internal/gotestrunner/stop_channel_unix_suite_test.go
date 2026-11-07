//go:build unix

package gotestrunner_test

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/mvrahden/go-test/internal/control"
	"github.com/mvrahden/go-test/internal/gotestrunner"
	"github.com/mvrahden/go-test/internal/testkit"
	"github.com/mvrahden/go-test/pkg/gotest"
)

// A shell stands in for a suite process here: $0 is its dir, descriptor 3
// its channel. It records what reaches it, line by line, in $0/got.
const (
	// shHello takes the channel.
	shHello = `echo "hello 1 $$" >&3; `
	// shTraps records SIGINT and SIGTERM, and lives on.
	shTraps = `trap 'echo SIGINT >> "$0/got"' INT; trap 'echo SIGTERM >> "$0/got"' TERM; `
	// shReady says the scenario may stop it.
	shReady = `touch "$0/ready"; `
	// shReadLines records each line the runner sends, ending on force.
	shReadLines = `while read -r l <&3; do echo "$l" >> "$0/got"; [ "$l" = force ] && exit 0; done; `
	// shIdle waits to be stopped, 15s at most.
	shIdle = `i=0; while [ $i -lt 300 ]; do sleep 0.05; i=$((i+1)); done`
)

// shRun is one scenario's stand-in suite process run by the runner.
type shRun struct {
	dir    string
	result gotestrunner.SuiteResult
	took   time.Duration // from the start to the run's cancel, then to its end
	cancel time.Duration
}

// runSh runs script as the suite process, cancelling the run once it is
// ready; within, when not zero, replaces the time it has to say hello.
func runSh(t *gotest.T, script string, within time.Duration, during func(dir string)) *shRun {
	if within > 0 {
		defer gotestrunner.ExportHelloWithin(within)()
	}
	r := &shRun{dir: t.TempDir()}
	target := gotestrunner.SuiteTarget{
		SuiteSpec:  gotestrunner.SuiteSpec{Package: "example.com/pkg", SuiteName: "TestFooSuite"},
		Exec:       []string{"/bin/sh", "-c", script, r.dir},
		BinaryPath: "suite.test",
	}
	started := time.Now()
	run, cancel := context.WithCancel(context.Background())
	acted := make(chan struct{})
	go func() {
		defer cancel()
		for deadline := time.Now().Add(30 * time.Second); time.Now().Before(deadline); time.Sleep(10 * time.Millisecond) {
			if _, err := os.Stat(filepath.Join(r.dir, "ready")); err == nil {
				r.cancel = time.Since(started)
				if during != nil {
					go func() {
						defer close(acted)
						during(r.dir)
					}()
				} else {
					close(acted)
				}
				return
			}
		}
		close(acted)
	}()
	r.result = gotestrunner.RunSingleSuite(run, target, nil, false)
	r.took = time.Since(started)
	<-acted
	return r
}

func (r *shRun) got() string {
	b, _ := os.ReadFile(filepath.Join(r.dir, "got")) //nolint:gosec // G304: the scenario's own dir
	return string(b)
}

// awaitGot waits until the stand-in recorded want.
func awaitGot(dir, want string) {
	for deadline := time.Now().Add(10 * time.Second); time.Now().Before(deadline); time.Sleep(10 * time.Millisecond) {
		if b, _ := os.ReadFile(filepath.Join(dir, "got")); strings.Contains(string(b), want) { //nolint:gosec // G304: the scenario's own dir
			return
		}
	}
}

// ChannelFallbackTestSuite covers what the runner sends a suite process
// that took its channel, and what it sends one that did not.
// Sequential: Abandon acts on every tree this process tracks, and the hello's
// bound is process-wide.
type ChannelFallbackTestSuite struct{}

func (s *ChannelFallbackTestSuite) TestAProcessThatTookItsChannelGetsTheLine(t *gotest.T) {
	// It leaves a process behind that ends on SIGTERM and says so.
	r := runSh(t, `(trap 'touch "$0/leftover"; exit 0' TERM; while :; do sleep 0.05; done) & `+
		shTraps+shHello+shReady+`read -r l <&3; echo "$l" >> "$0/got"; exit 0`, 0, nil)
	_, err := os.Stat(filepath.Join(r.dir, "leftover"))

	t.When("the run is cancelled and the suite process took its channel", func(w *gotest.T) {
		w.It("sends it the stop line and no signal", func(it *gotest.T) {
			gotest.Equal(it, "stop\n", r.got())
			gotest.True(it, r.result.CutShort)
		})
		w.It("asks what it left running to stop once it has exited", func(it *gotest.T) {
			gotest.NoError(it, err, "the leftover process got no SIGTERM")
		})
	})
}

func (s *ChannelFallbackTestSuite) TestAProcessThatSaidNoHelloGetsCtrlC(t *gotest.T) {
	// Says it was stopped by the very signal the runner sent.
	r := runSh(t, `trap 'echo SIGINT >> "$0/got"; echo "stopped signal:SIGINT" >&3; exit 0' INT; `+
		`trap 'echo SIGTERM >> "$0/got"' TERM; `+shReady+shIdle, 500*time.Millisecond, nil)

	t.When("the run is cancelled and the suite process says no hello within the bound", func(w *gotest.T) {
		w.It("interrupts its group once, with SIGINT, once the bound has passed", func(it *gotest.T) {
			gotest.Equal(it, "SIGINT\n", r.got())
			gotest.GreaterOrEqual(it, r.took, 500*time.Millisecond)
			gotest.Less(it, r.took, 10*time.Second)
		})
		w.It("does not book the stop the runner sent as one from outside", func(it *gotest.T) {
			gotest.True(it, r.result.CutShort)
			gotest.Empty(it, r.result.StoppedBy)
		})
	})
}

func (s *ChannelFallbackTestSuite) TestAHelloWithinTheBoundIsWaitedFor(t *gotest.T) {
	// Ready at once, hello only 300ms later.
	r := runSh(t, shTraps+shReady+`sleep 0.3; `+shHello+`read -r l <&3; echo "$l" >> "$0/got"; exit 0`, 3*time.Second, nil)

	t.When("the run is cancelled before the suite process said its hello, which comes within the bound", func(w *gotest.T) {
		w.It("waits for the hello and sends the stop line, not a signal", func(it *gotest.T) {
			gotest.Equal(it, "stop\n", r.got())
		})
	})
}

func (s *ChannelFallbackTestSuite) TestALineThatCannotBeWrittenFallsBack(t *gotest.T) {
	// Closes its channel once it said hello.
	r := runSh(t, shTraps+shHello+`exec 3>&-; trap 'echo SIGINT >> "$0/got"; exit 0' INT; `+shReady+shIdle, 0, nil)

	t.When("the run is cancelled and the stop line cannot be written to a suite process that said hello", func(w *gotest.T) {
		w.It("interrupts its group with SIGINT instead", func(it *gotest.T) {
			gotest.Equal(it, "SIGINT\n", r.got())
			gotest.True(it, r.result.CutShort)
		})
	})
}

func (s *ChannelFallbackTestSuite) TestTheBudgetLineGovernsTheGrace(t *gotest.T) {
	link, child, err := control.New()
	gotest.NoError(t, err)
	// Reports two budgets, then ignores every stop.
	cmd := exec.Command("/bin/sh", "-c", `printf 'hello 1 1\nbudget 600ms\nbudget 200ms\n' >&3; trap '' INT TERM; echo ready; sleep 30`) //nolint:gosec // G204: fixed script
	child.Attach(cmd)
	stdout, err := cmd.StdoutPipe()
	gotest.NoError(t, err)
	mp := gotestrunner.NewManagedProcess(cmd, gotestrunner.ProcessConfig{Grace: gotestrunner.GraceBudget, Link: link})
	err = mp.Start()
	child.Close()
	gotest.NoError(t, err)
	buf := make([]byte, 64)
	n, _ := stdout.Read(buf)
	gotest.Contains(t, string(buf[:n]), "ready")
	time.Sleep(200 * time.Millisecond) // the lines are read by then
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	started := time.Now()
	_ = mp.WaitWithGrace(ctx)
	took := time.Since(started)

	t.When("a suite process reports teardown budgets on its channel and outlives its stop", func(w *gotest.T) {
		w.It("is given the largest of them, then killed", func(it *gotest.T) {
			gotest.GreaterOrEqual(it, took, 600*time.Millisecond)
			gotest.Less(it, took, testkit.Scaled(3*time.Second))
		})
	})
}

func (s *ChannelFallbackTestSuite) TestASecondStopIsTheForceLine(t *gotest.T) {
	var took time.Duration
	r := runSh(t, `trap '' INT TERM; `+shHello+shReady+shReadLines+shIdle, 0, func(dir string) {
		awaitGot(dir, "stop")
		started := time.Now()
		gotestrunner.Abandon(5*time.Second, nil)
		took = time.Since(started)
	})

	t.When("the CLI gives up on a suite process it stopped by its channel", func(w *gotest.T) {
		w.It("sends it the force line after the stop line, and no signal", func(it *gotest.T) {
			gotest.Equal(it, "stop\nforce\n", r.got())
		})
		w.It("waits for it to end rather than killing it", func(it *gotest.T) {
			gotest.Less(it, took, 3*time.Second)
			gotest.Less(it, r.took, 10*time.Second)
		})
	})
}

func (s *ChannelFallbackTestSuite) TestASecondStopKillsAProcessThatSaidNoHello(t *gotest.T) {
	var took time.Duration
	r := runSh(t, `trap 'echo SIGINT >> "$0/got"' INT; trap '' TERM; `+shReady+shIdle, 200*time.Millisecond, func(dir string) {
		awaitGot(dir, "SIGINT")
		started := time.Now()
		gotestrunner.Abandon(5*time.Second, nil)
		took = time.Since(started)
	})

	t.When("the CLI gives up on a suite process that never took its channel", func(w *gotest.T) {
		w.It("kills it at once", func(it *gotest.T) {
			gotest.Equal(it, "SIGINT\n", r.got())
			gotest.Less(it, took, 2*time.Second)
		})
	})
}

func (s *ChannelStopTestSuite) TestASignalFromOutsideIsBooked(t *gotest.T) {
	if os.Getenv(envChannelMode) != "" {
		channelChildRun(t.T())
		return
	}
	r := runChannelSuite(t, "TestASignalFromOutsideIsBooked", channelTerm, nil, false)
	collector := gotestrunner.NewOutputCollector(gotestrunner.RunCaptureJSON, false)
	collector.Register("example.com/stop", 1)
	collector.RecordResult("example.com/stop", 0, r.result)

	t.When("a suite process on its channel is sent SIGTERM from outside the runner", func(w *gotest.T) {
		w.It("tells the runner, which records who stopped it", func(it *gotest.T) {
			gotest.Equal(it, "signal:SIGTERM", r.result.StoppedBy, string(r.result.Stderr))
			gotest.False(it, r.result.CutShort)
		})
		w.It("fails its package, naming the signal in the stream", func(it *gotest.T) {
			gotest.Equal(it, 1, collector.WorstExitCode())
			gotest.Contains(it, string(collector.CapturedJSON()), "gotest: suite stopped by SIGTERM from outside the runner")
		})
	})
}

func (s *ChannelStopTestSuite) TestAForkingWrapperLeavesTheSuiteRunning(t *gotest.T) {
	if os.Getenv(envChannelMode) != "" {
		channelChildRun(t.T())
		return
	}
	// Starts the suite process in the background and exits at once.
	r := runChannelSuite(t, "TestAForkingWrapperLeavesTheSuiteRunning", channelSleep, []string{"/bin/sh", "-c", `"$@" & exit 0`, "sh"}, false)
	verdict, _ := r.verdicts("TestAForkingWrapperLeavesTheSuiteRunning")

	t.When("the -exec command exits while the suite process it started still runs", func(w *gotest.T) {
		w.It("keeps the channel open until the tree is released, so the suite runs to its end", func(it *gotest.T) {
			for _, m := range []string{"TestFirst", "TestSecond", "TestThird"} {
				gotest.Equal(it, "pass", verdict[m], string(r.result.Stdout))
			}
			gotest.Empty(it, r.result.StoppedBy)
		})
	})
}
