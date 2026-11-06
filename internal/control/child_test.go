package control_test

import (
	"bufio"
	"bytes"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/mvrahden/go-test/internal/control"
)

// envChild makes a run of this test binary one of the child programs below.
const envChild = "GOTEST_CONTROL_TEST_CHILD"

const (
	// envChannel carries GOTEST_CONTROL to a child under another name: a
	// runtime that claims the channel at init would otherwise take it before
	// the code under test runs.
	envChannel = "GOTEST_CONTROL_TEST_CHANNEL"
	envDir     = "GOTEST_CONTROL_TEST_DIR"
	envReport  = "GOTEST_CONTROL_TEST_REPORT"
	envExpect  = "GOTEST_CONTROL_TEST_EXPECT"
	// envLimit lowers the child's descriptor limit before it runs its mode.
	envLimit = "GOTEST_CONTROL_TEST_LIMIT"
)

func TestMain(m *testing.M) {
	if mode := os.Getenv(envChild); mode != "" {
		os.Exit(runChild(mode))
	}
	os.Exit(m.Run())
}

// runChild is a child program. Each reports on stdout as key=value lines.
//   - roundtrip: says hello, budget and stopped, then reads two lines
//   - wait: says hello, then reads until the channel ends
//   - deadline: a read cut short by its stop, then a read that gets a line
//   - drain: drains once before hello and once per line on stdin; on busy,
//     while a Next reads
//   - spawn: starts probes in every way a test starts a process
//   - probe: reports whether it holds the channel, then lingers
//   - claim: claims whatever it was given
//   - claimstd: claims its own standard streams
func runChild(mode string) int {
	if v, ok := os.LookupEnv(envChannel); ok {
		_ = os.Unsetenv(envChannel)
		_ = os.Setenv(control.Env, v)
	}
	if limit := os.Getenv(envLimit); limit != "" && lowerLimit(limit) != nil {
		return 2
	}
	switch mode {
	case "roundtrip":
		return roundTrip()
	case "wait":
		return waitForEnd()
	case "deadline":
		return deadline()
	case "drain":
		return drain()
	case "spawn":
		return spawn()
	case "probe":
		return probe()
	case "claim":
		return claimOnly()
	case "claimstd":
		_ = os.Setenv(control.Env, standardValue())
		return claimOnly()
	}
	return 2
}

func report(key string, value any) { fmt.Printf("%s=%v\n", key, value) }

func claimed() *control.Endpoint {
	e := control.Claim()
	if e == nil {
		report("claim", "nil")
	}
	return e
}

func roundTrip() int {
	e := claimed()
	if e == nil {
		return 2
	}
	// None of these is a line the runner keeps.
	if e.WriteRaw([]byte("stop\nbogus 1\nhello 1\n")) != nil {
		return 2
	}
	if e.Hello(os.Getpid()) != nil || e.Budget(5*time.Minute+30*time.Second) != nil || e.Stopped("signal:SIGTERM") != nil {
		return 2
	}
	first, err := e.Next(nil)
	report("first", outcome(first, err))
	second, err := e.Next(nil)
	report("second", outcome(second, err))
	return 0
}

func waitForEnd() int {
	e := claimed()
	if e == nil || e.Hello(os.Getpid()) != nil {
		return 2
	}
	l, err := e.Next(nil)
	report("next", outcome(l, err))
	return 0
}

func deadline() int {
	e := claimed()
	if e == nil || e.Hello(os.Getpid()) != nil {
		return 2
	}
	stop := make(chan struct{})
	var closedAt atomic.Int64
	time.AfterFunc(50*time.Millisecond, func() {
		closedAt.Store(time.Now().UnixNano())
		close(stop)
	})
	l, err := e.Next(stop)
	report("first.after", time.Since(time.Unix(0, closedAt.Load())).Nanoseconds())
	report("first", outcome(l, err))
	report("first.leaked", leaked())
	// The runner answers this with a line.
	if e.Budget(time.Second) != nil {
		return 2
	}
	l, err = e.Next(make(chan struct{}))
	report("second", outcome(l, err))
	report("second.leaked", leaked())
	return 0
}

func drain() int {
	e := claimed()
	if e == nil {
		return 2
	}
	step := func(name string) {
		lines, eof := e.Drain()
		report(name, kinds(lines))
		report(name+".eof", eof)
	}
	step("before")
	if e.Hello(os.Getpid()) != nil {
		return 2
	}
	in := bufio.NewScanner(os.Stdin)
	for in.Scan() {
		if in.Text() != "busy" {
			step(in.Text())
			continue
		}
		// A Drain while a Next reads. Reported only once that Next has
		// ended: a line the runner sends on the report would otherwise be
		// the Next's to take, not the next Drain's.
		stop, done := make(chan struct{}), make(chan struct{})
		go func() {
			_, _ = e.Next(stop)
			close(done)
		}()
		time.Sleep(50 * time.Millisecond)
		start := time.Now()
		lines, eof := e.Drain()
		took := time.Since(start)
		close(stop)
		<-done
		report("busy", kinds(lines))
		report("busy.eof", eof)
		report("busy.took", took.Nanoseconds())
	}
	return 0
}

// spawn starts a probe in every way of this platform and passes on what each
// reports, prefixed with the way.
func spawn() int {
	raw := os.Getenv(control.Env)
	ids := identify(raw)
	e := claimed()
	if e == nil || e.Hello(os.Getpid()) != nil {
		return 2
	}
	_, inEnv := os.LookupEnv(control.Env)
	report("env", inEnv)
	expect := reportClaimed(e, raw, ids)
	for _, w := range spawnWays {
		file := filepath.Join(os.Getenv(envDir), w.name)
		env := append(baseEnv("probe"), envReport+"="+file, envExpect+"="+expect)
		if err := w.start(env); err != nil {
			report(w.name+".error", err)
			continue
		}
		data, err := awaitFile(file)
		if err != nil {
			report(w.name+".error", err)
			continue
		}
		for line := range strings.Lines(string(data)) {
			fmt.Print(w.name + "." + line)
		}
	}
	report("done", true)
	return 0
}

// probe reports whether it holds the channel and whether the variable reached
// it, then stays until killed.
func probe() int {
	_, inEnv := os.LookupEnv(control.Env)
	body := fmt.Sprintf("pid=%d\nenv=%v\nheld=%s\n", os.Getpid(), inEnv, held(os.Getenv(envExpect)))
	file := os.Getenv(envReport)
	if os.WriteFile(file+".tmp", []byte(body), 0o600) != nil || os.Rename(file+".tmp", file) != nil {
		return 2
	}
	time.Sleep(30 * time.Second)
	return 0
}

func claimOnly() int {
	raw := os.Getenv(control.Env)
	e := control.Claim()
	_, inEnv := os.LookupEnv(control.Env)
	report("claimed", e != nil)
	report("env", inEnv)
	report("untouched", untouched(raw))
	return 0
}

// baseEnv is this process's environment running mode, without the variables
// that configured this process.
func baseEnv(mode string) []string {
	env := []string{envChild + "=" + mode}
	for _, kv := range os.Environ() {
		if !strings.HasPrefix(kv, "GOTEST_CONTROL_TEST_") {
			env = append(env, kv)
		}
	}
	return env
}

func awaitFile(path string) ([]byte, error) {
	end := time.Now().Add(10 * time.Second)
	for {
		data, err := os.ReadFile(path)
		if err == nil || time.Now().After(end) {
			return data, err
		}
		time.Sleep(10 * time.Millisecond)
	}
}

// outcome names a line's kind, or the error that came instead.
func outcome(l control.Line, err error) string {
	switch {
	case errors.Is(err, io.EOF):
		return "eof"
	case errors.Is(err, control.ErrStopped):
		return "stopped"
	case err != nil:
		return "error:" + err.Error()
	}
	return l.Kind.String()
}

func kinds(lines []control.Line) string {
	if len(lines) == 0 {
		return "-"
	}
	names := make([]string, len(lines))
	for i, l := range lines {
		names[i] = l.Kind.String()
	}
	return strings.Join(names, ",")
}

// leaked reports whether a goroutine of the endpoint is still running.
// leaked reports whether a goroutine of the endpoint's is still running. A
// goroutine joined through a channel can still show in the stack dump for
// an instant after, so it looks again for a moment before saying so.
func leaked() bool {
	buf := make([]byte, 1<<20)
	for deadline := time.Now().Add(200 * time.Millisecond); ; {
		n := runtime.Stack(buf, true)
		if !bytes.Contains(buf[:n], []byte("control.(*Endpoint)")) {
			return false
		}
		if time.Now().After(deadline) {
			return true
		}
		time.Sleep(time.Millisecond)
	}
}

// way is one way a test starts a process.
type way struct {
	name  string
	start func(env []string) error
}
