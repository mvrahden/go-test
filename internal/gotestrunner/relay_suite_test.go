package gotestrunner_test

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/signal"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/mvrahden/go-test/internal/gotestrunner"
	"github.com/mvrahden/go-test/internal/proctree"
	"github.com/mvrahden/go-test/internal/testkit"
	"github.com/mvrahden/go-test/pkg/gotest"
)

// A run of this test binary with envRelayRaw set is a suite on a JSON
// pipeline: it writes the stream the named file holds to stdout and exits
// with envRelayStatus. With envRelayHold naming a directory it writes the
// stream to the line past its half, publishes "<pid> <ppid>" there as
// halfway, catches SIGINT and SIGTERM and lists them in signals, and writes
// the rest once release appears there, 20s at most, a line at a time.
const (
	envRelayRaw    = "GOTESTRUNNER_RELAY_RAW"
	envRelayStatus = "GOTESTRUNNER_RELAY_STATUS"
	envRelayHold   = "GOTESTRUNNER_RELAY_HOLD"
)

// TestMain runs the stand-in suite, not init: Windows calls a console
// control handler on a thread of its own, which Go holds until package
// initialization has ended, so a stand-in running in init never sees a
// CTRL_BREAK. It exits past the test binary's exit hooks, which by now
// include coverage's: its output is the stream alone.
func TestMain(m *testing.M) {
	if raw := os.Getenv(envRelayRaw); raw != "" {
		syscall.Exit(relayedSuite(raw, os.Getenv(envRelayStatus), os.Getenv(envRelayHold)))
	}
	os.Exit(m.Run())
}

func relayedSuite(path, status, hold string) int {
	code, _ := strconv.Atoi(status)
	raw, err := os.ReadFile(path) //nolint:gosec // G304: the scenario's own stream
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 99
	}
	if hold == "" {
		_, _ = os.Stdout.Write(raw)
		return code
	}
	got := make(chan os.Signal, 2)
	signal.Notify(got, os.Interrupt, syscall.SIGTERM)
	half := len(raw)/2 + bytes.IndexByte(raw[len(raw)/2:], '\n') + 1
	_, _ = os.Stdout.Write(raw[:half])
	publish(filepath.Join(hold, "halfway"), fmt.Sprintf("%d %d", os.Getpid(), os.Getppid()))
	var caught []string
	tick := time.NewTicker(10 * time.Millisecond)
	defer tick.Stop()
	for deadline := time.Now().Add(20 * time.Second); time.Now().Before(deadline); {
		select {
		case sig := <-got:
			caught = append(caught, sig.String())
			publish(filepath.Join(hold, "signals"), strings.Join(caught, ","))
			continue
		case <-tick.C:
		}
		if _, err := os.Stat(filepath.Join(hold, "release")); err == nil {
			break
		}
	}
	for line := range strings.Lines(string(raw[half:])) {
		_, _ = os.Stdout.WriteString(line)
		time.Sleep(time.Millisecond)
	}
	return code
}

// publish writes a file whole, by rename, so no reader sees a part of it.
func publish(path, content string) {
	if os.WriteFile(path+".tmp", []byte(content), 0o600) == nil {
		_ = os.Rename(path+".tmp", path)
	}
}

// relayed runs the stream in raw as a suite of pkg on a JSON pipeline.
func relayed(ctx context.Context, raw string, status int, pkg string, env ...string) gotestrunner.SuiteResult {
	target := gotestrunner.SuiteTarget{
		SuiteSpec:  gotestrunner.SuiteSpec{Package: pkg, SuiteName: "TestRelayed"},
		BinaryPath: os.Args[0],
	}
	env = append(append(os.Environ(), envRelayRaw+"="+raw, envRelayStatus+"="+strconv.Itoa(status)), env...)
	return gotestrunner.RunSingleSuite(ctx, target, env, true)
}

// relayRaw is the committed raw stream of a case, by absolute path.
func relayRaw(t *gotest.T, name string) string {
	return gotest.Must(filepath.Abs(filepath.Join("testdata", "relay", name+".raw")))
}

// relayGolden is the committed conversion of a case.
func relayGolden(t *gotest.T, name string) []string {
	return eventsOf(t, gotest.Must(os.ReadFile(filepath.Join("testdata", "relay", name+".golden"))))
}

// eventsOf reads a stream of events, each re-encoded without its Time, and a
// package event without its Elapsed too: those are the clock's; every test
// in the corpus took 0.00s. The OutputType and the error markers in Output
// go too: older converters know neither, and the goldens hold for every Go
// the gate runs.
func eventsOf(t *gotest.T, stream []byte) []string {
	var out []string
	for line := range strings.Lines(string(stream)) {
		var ev map[string]any
		gotest.NoError(t, json.Unmarshal([]byte(line), &ev), "no event: %q", line)
		delete(ev, "Time")
		delete(ev, "OutputType")
		if output, ok := ev["Output"].(string); ok {
			ev["Output"] = strings.NewReplacer("\x0e", "", "\x0f", "").Replace(output)
		}
		if ev["Test"] == nil {
			delete(ev, "Elapsed")
		}
		out = append(out, string(gotest.Must(json.Marshal(ev))))
	}
	return out
}

// relayEvent is the part of an event the crash tail scenarios read.
type relayEvent struct{ Action, Test, Output string }

func decodeEvents(t *gotest.T, stream []byte) []relayEvent {
	var out []relayEvent
	for line := range strings.Lines(string(stream)) {
		var ev relayEvent
		gotest.NoError(t, json.Unmarshal([]byte(line), &ev), "no event: %q", line)
		out = append(out, ev)
	}
	return out
}

// verdictsOf maps each test that got a verdict to it.
func verdictsOf(events []relayEvent) map[string]string {
	verdicts := map[string]string{}
	for _, ev := range events {
		switch ev.Action {
		case "pass", "fail", "skip":
			if ev.Test != "" {
				verdicts[ev.Test] = ev.Action
			}
		}
	}
	return verdicts
}

// RelayTestSuite covers the converter beside a suite on a JSON pipeline: it
// converts the suite's own stream as the toolchain's converter does, and ends
// one the suite left without its final result line.
type RelayTestSuite struct{}

func (s *RelayTestSuite) SuiteConfig() gotest.SuiteConfig {
	cfg := gotest.DefaultSuiteConfig()
	cfg.Parallel = true
	return cfg
}

func (s *RelayTestSuite) TestAStreamConvertsEventForEvent(t *gotest.T) {
	for t, c := range gotest.Each(t, []struct {
		Name   string
		status int
	}{{"passing", 0}, {"failing", 1}, {"skipping", 0}, {"subtests", 1}, {"panicking", 2}, {"exiting", 3}}) {
		result := relayed(t.Context(), relayRaw(t, c.Name), c.status, "example.com/relay/"+c.Name)
		gotest.Equal(t, relayGolden(t, c.Name), eventsOf(t, result.Stdout))
		gotest.Empty(t, result.Stderr)
		gotest.Equal(t, min(c.status, 1), result.ExitCode, "a status the binary chose fails the suite as 1")
	}
}

func (s *RelayTestSuite) TestTheResultCarriesTheSuitesOwnStatus(t *gotest.T) {
	result := relayed(t.Context(), relayRaw(t, "passing"), 66, "example.com/relay/passing")
	t.When("the suite exits 66 after its final PASS", func(w *gotest.T) {
		w.It("reports 66, the status the race detector leaves", func(it *gotest.T) {
			gotest.Equal(it, 66, result.ExitCode)
		})
		w.It("converts the stream as the binary printed it", func(it *gotest.T) {
			gotest.Equal(it, relayGolden(it, "passing"), eventsOf(it, result.Stdout))
		})
	})
}

func (s *RelayTestSuite) TestAStreamCutMidTest(t *gotest.T) {
	raw := gotest.Must(os.ReadFile(relayRaw(t, "subtests")))
	report := []byte("--- FAIL: TestTree/branch/deep_fail (0.00s)\n")
	cut := bytes.Index(raw, report)
	gotest.Greater(t, cut, 0)
	stream := slices.Concat(raw[:cut+len(report)], []byte("    cut mid-line"))
	path := filepath.Join(t.TempDir(), "cut.raw")
	gotest.NoError(t, os.WriteFile(path, stream, 0o600))
	result := relayed(t.Context(), path, 2, "example.com/relay/cut")
	events := decodeEvents(t, result.Stdout)
	verdicts := verdictsOf(events)
	gotest.NotEmpty(t, events)

	t.When("the suite exits 2 with its stream cut mid-test", func(w *gotest.T) {
		w.It("ends the stream with a final FAIL", func(it *gotest.T) {
			gotest.Equal(it, relayEvent{Action: "fail"}, events[len(events)-1])
		})
		w.It("releases every verdict the converter held back", func(it *gotest.T) {
			gotest.Equal(it, map[string]string{"TestTree/leaf_pass": "pass", "TestTree/branch/deep_fail": "fail"}, verdicts)
		})
		w.It("leaves the running tests without a verdict", func(it *gotest.T) {
			gotest.NotContains(it, verdicts, "TestTree")
			gotest.NotContains(it, verdicts, "TestTree/branch")
		})
		w.It("attributes the FAIL it adds to no test", func(it *gotest.T) {
			var on []string
			for _, ev := range events {
				if ev.Output == "FAIL\n" {
					on = append(on, ev.Test)
				}
			}
			gotest.Equal(it, []string{""}, on)
		})
		w.It("keeps the cut line as the binary wrote it", func(it *gotest.T) {
			gotest.Contains(it, events, relayEvent{Action: "output", Test: "TestTree/branch/deep_fail", Output: "    cut mid-line\n"})
		})
		w.It("fails the suite", func(it *gotest.T) {
			gotest.Equal(it, 1, result.ExitCode)
		})
	})
}

func (s *RelayTestSuite) TestAStreamThatExitsZeroWithoutItsResult(t *gotest.T) {
	raw := gotest.Must(os.ReadFile(relayRaw(t, "passing")))
	stream, found := bytes.CutSuffix(raw, []byte("\x16PASS\n"))
	gotest.True(t, found)
	path := filepath.Join(t.TempDir(), "noresult.raw")
	gotest.NoError(t, os.WriteFile(path, stream, 0o600))
	result := relayed(t.Context(), path, 0, "example.com/relay/noresult")
	events := decodeEvents(t, result.Stdout)
	gotest.NotEmpty(t, events)

	t.When("the suite exits 0 without its final result line, as a TestMain that skips m.Run", func(w *gotest.T) {
		w.It("ends the stream with a final PASS", func(it *gotest.T) {
			gotest.Equal(it, relayEvent{Action: "pass"}, events[len(events)-1])
		})
		w.It("keeps every verdict", func(it *gotest.T) {
			gotest.Equal(it, map[string]string{"TestOne": "pass", "TestTwo": "pass"}, verdictsOf(events))
		})
		w.It("passes the suite", func(it *gotest.T) {
			gotest.Zero(it, result.ExitCode)
		})
	})
}

// RelayConverterTestSuite covers the converter as a process of its own: one
// that dies mid-run is replaced, and a stop aimed at the suite misses it.
// Sequential: each method reads the converters started through one hook.
type RelayConverterTestSuite struct{}

type converterCtx struct {
	started chan *os.Process
	restore func()
	hold    string
}

func (s *RelayConverterTestSuite) BeforeEach(t *gotest.T) *converterCtx {
	started := make(chan *os.Process, 4)
	restore := gotestrunner.ExportOnConverterStart(func(p *os.Process) { started <- p })
	return &converterCtx{started: started, restore: restore, hold: t.TempDir()}
}

func (s *RelayConverterTestSuite) AfterEach(t *gotest.T, ctx *converterCtx) { ctx.restore() }

// converter waits for the converter the relay started.
func (ctx *converterCtx) converter(t *gotest.T) *os.Process {
	select {
	case p := <-ctx.started:
		return p
	case <-time.After(10 * time.Second):
		gotest.Fail(t, "no converter started")
	}
	return nil
}

// halfway waits until the suite wrote half its stream and reads its pid and
// parent's pid.
func (ctx *converterCtx) halfway(t *gotest.T) (pid, ppid int) {
	var content []byte
	gotest.Eventually(t, 10*time.Second, 10*time.Millisecond, func(poll *gotest.R) {
		var err error
		content, err = os.ReadFile(filepath.Join(ctx.hold, "halfway"))
		gotest.NoError(poll, err)
	})
	_, err := fmt.Sscanf(string(content), "%d %d", &pid, &ppid)
	gotest.NoError(t, err)
	return pid, ppid
}

func (ctx *converterCtx) release(t *gotest.T) {
	gotest.NoError(t, os.WriteFile(filepath.Join(ctx.hold, "release"), nil, 0o600))
}

func (s *RelayConverterTestSuite) TestAConverterKilledMidRunIsReplaced(t *gotest.T, ctx *converterCtx) {
	var result gotestrunner.SuiteResult
	wait := gotest.Go(t, func() {
		result = relayed(t.Context(), relayRaw(t, "subtests"), 1, "example.com/relay/subtests", envRelayHold+"="+ctx.hold)
	})
	converter := ctx.converter(t)
	ctx.halfway(t)
	gotest.NoError(t, converter.Kill())
	gotest.Eventually(t, 10*time.Second, 10*time.Millisecond, func(poll *gotest.R) {
		gotest.False(poll, proctree.Alive(converter.Pid))
	})
	ctx.release(t)
	wait()

	t.When("the converter is killed while the suite is halfway through its stream", func(w *gotest.T) {
		w.It("converts the whole stream again", func(it *gotest.T) {
			gotest.Equal(it, relayGolden(it, "subtests"), eventsOf(it, result.Stdout))
		})
		w.It("says so on the suite's stderr", func(it *gotest.T) {
			gotest.Regexp(it, `^gotest: example\.com/relay/subtests TestRelayed: the converter ended early \((signal: killed|exit status 1)\); the suite's output was converted again\n$`, string(result.Stderr))
		})
	})
}

func (s *RelayConverterTestSuite) TestAStopAimedAtTheSuiteMissesTheConverter(t *gotest.T, ctx *converterCtx) {
	run, cancel := context.WithCancel(t.Context())
	defer cancel()
	var result gotestrunner.SuiteResult
	wait := gotest.Go(t, func() {
		result = relayed(run, relayRaw(t, "subtests"), 1, "example.com/relay/subtests", envRelayHold+"="+ctx.hold)
	})
	converter := ctx.converter(t)
	ctx.halfway(t)
	// No stop file: the run interrupts the suite's tree, SIGTERM to its
	// group on Unix, CTRL_BREAK on its console on Windows.
	cancel()
	// Scaled: on Windows the Ctrl-Break goes through a helper run of this
	// executable, whose first start can wait on an antivirus scan.
	gotest.Eventually(t, testkit.Scaled(20*time.Second), 10*time.Millisecond, func(poll *gotest.R) {
		_, err := os.Stat(filepath.Join(ctx.hold, "signals"))
		gotest.NoError(poll, err)
	})
	running := proctree.Alive(converter.Pid)
	ctx.release(t)
	wait()

	t.When("the run interrupts the suite's tree while the suite is halfway through its stream", func(w *gotest.T) {
		w.It("reaches the suite", func(it *gotest.T) {
			gotest.True(it, result.CutShort)
		})
		w.It("leaves the converter running", func(it *gotest.T) {
			gotest.True(it, running)
		})
		w.It("converts the stream once, whole", func(it *gotest.T) {
			gotest.Equal(it, relayGolden(it, "subtests"), eventsOf(it, result.Stdout))
			gotest.Empty(it, result.Stderr)
		})
	})
}
