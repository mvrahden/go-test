package drill_test

import (
	"bytes"
	"encoding/json"
	"errors"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/mvrahden/go-test/internal/drill"
	"github.com/mvrahden/go-test/pkg/gotest"
)

// started is a child this test started and waits for.
type started struct {
	cmd  *exec.Cmd
	done chan struct{}
}

// procs holds one test's directory and the processes it started.
type procs struct {
	dir string

	mu      sync.Mutex
	started []*started
	pids    []int // published by descendants
}

func newProcs(t *gotest.T) *procs { return &procs{dir: t.TempDir()} }

func (p *procs) start(t *gotest.T, cmd *exec.Cmd) *started {
	gotest.NoError(t, cmd.Start())
	s := &started{cmd: cmd, done: make(chan struct{})}
	go func() {
		_ = cmd.Wait()
		close(s.done)
	}()
	p.mu.Lock()
	p.started = append(p.started, s)
	p.mu.Unlock()
	return s
}

// wait returns the child's exit code once it ended, which it must within limit.
func (s *started) wait(t *gotest.T, limit time.Duration) int {
	select {
	case <-s.done:
	case <-time.After(limit):
		gotest.Fail(t, "the child did not end within %s", limit)
	}
	return s.cmd.ProcessState.ExitCode()
}

// pid reads the pid a descendant published at path.
func (p *procs) pid(t *gotest.T, path string) int {
	gotest.Eventually(t, 30*time.Second, 10*time.Millisecond, func(poll *gotest.R) {
		_, err := os.Stat(path)
		gotest.NoError(poll, err, "the child never started")
	})
	data, err := os.ReadFile(path)
	gotest.NoError(t, err)
	pid, err := strconv.Atoi(string(data))
	gotest.NoError(t, err)
	p.mu.Lock()
	p.pids = append(p.pids, pid)
	p.mu.Unlock()
	return pid
}

// stop kills what this test started and is still there: its children, and
// published descendants still running this test binary.
func (p *procs) stop() {
	for _, s := range p.started {
		select {
		case <-s.done:
		default:
			_ = s.cmd.Process.Kill()
			<-s.done
		}
	}
	for _, pid := range p.pids {
		cmdline, err := os.ReadFile("/proc/" + strconv.Itoa(pid) + "/cmdline")
		if err == nil && bytes.HasPrefix(cmdline, []byte(self()+"\x00")) {
			if proc, err := os.FindProcess(pid); err == nil {
				_ = proc.Kill()
			}
		}
	}
}

// gone reports whether no process has pid, not even a zombie.
func gone(pid int) bool {
	_, err := os.Stat("/proc/" + strconv.Itoa(pid))
	return errors.Is(err, fs.ErrNotExist)
}

// SupervisorTestSuite: a supervisor bounds its command and leaves nothing of
// it behind, whichever way the command ends.
type SupervisorTestSuite struct{}

func (s *SupervisorTestSuite) SuiteGuard() string {
	if runtime.GOOS != "linux" {
		return "the drill runs on Linux only"
	}
	return ""
}

func (s *SupervisorTestSuite) SuiteConfig() gotest.SuiteConfig {
	cfg := gotest.IntegrationSuiteConfig()
	cfg.Parallel = true
	return cfg
}

func (s *SupervisorTestSuite) BeforeEach(t *gotest.T) *procs { return newProcs(t) }

func (s *SupervisorTestSuite) AfterEach(t *gotest.T, p *procs) { p.stop() }

// supervise runs the child program mode under a supervisor bounded by
// seconds, sends the supervisor interrupt, if any, once the descendant mode
// starts published its pid, and requires that descendant gone.
func supervise(t *gotest.T, p *procs, mode, seconds string, interrupt syscall.Signal) (int, drill.Status) {
	status, pidfile := filepath.Join(p.dir, "status"), filepath.Join(p.dir, "pid")
	supervisor := p.start(t, childCommand("drill", "--supervise", seconds, status, self(), mode, pidfile))
	child := p.pid(t, pidfile)
	if interrupt != 0 {
		gotest.NoError(t, supervisor.cmd.Process.Signal(interrupt))
	}
	code := supervisor.wait(t, time.Minute)
	data, err := os.ReadFile(status)
	gotest.NoError(t, err)
	var observed drill.Status
	gotest.NoError(t, json.Unmarshal(data, &observed))
	gotest.True(t, gone(child), "descendant %d outlived its supervisor", child)
	return code, observed
}

func (s *SupervisorTestSuite) TestANormalExitReapsADoubleForkedDetachedChild(t *gotest.T, p *procs) {
	code, status := supervise(t, p, "detach", "30", 0)
	gotest.Zero(t, code)
	gotest.NotZero(t, status.Exit)
	gotest.Zero(t, *status.Exit)
	gotest.False(t, status.Timeout)
}

func (s *SupervisorTestSuite) TestATimeoutCleansAChildInASeparateSession(t *gotest.T, p *procs) {
	code, status := supervise(t, p, "session", "2", 0)
	gotest.NotZero(t, code)
	gotest.True(t, status.Timeout)
}

func (s *SupervisorTestSuite) TestCancellationCleansDescendants(t *gotest.T, p *procs) {
	code, status := supervise(t, p, "session", "30", syscall.SIGTERM)
	gotest.NotZero(t, code)
	gotest.True(t, status.Interrupted)
}

func (s *SupervisorTestSuite) TestAHangupCancelsLikeATermination(t *gotest.T, p *procs) {
	code, status := supervise(t, p, "session", "30", syscall.SIGHUP)
	gotest.NotZero(t, code)
	gotest.True(t, status.Interrupted)
}

func (s *SupervisorTestSuite) TestAHangupIgnoredAtStartIsStillIgnored(t *gotest.T, p *procs) {
	status, pidfile := filepath.Join(p.dir, "status"), filepath.Join(p.dir, "pid")
	supervisor := p.start(t, underNohup(t, childCommand("drill", "--supervise", "30", status, self(), "session", pidfile)))
	p.pid(t, pidfile)
	gotest.NoError(t, supervisor.cmd.Process.Signal(syscall.SIGHUP))
	survives(t, supervisor, "a supervisor started under nohup stopped on SIGHUP")
	gotest.NoError(t, supervisor.cmd.Process.Signal(syscall.SIGTERM))
	supervisor.wait(t, time.Minute)
	data, err := os.ReadFile(status)
	gotest.NoError(t, err)
	var observed drill.Status
	gotest.NoError(t, json.Unmarshal(data, &observed))
	gotest.True(t, observed.Interrupted)
}

// underNohup runs cmd with SIGHUP ignored, as nohup starts it.
func underNohup(t *gotest.T, cmd *exec.Cmd) *exec.Cmd {
	nohup, err := exec.LookPath("nohup")
	gotest.NoError(t, err)
	wrapped := exec.Command(nohup, append([]string{cmd.Path}, cmd.Args[1:]...)...) //nolint:gosec // G204: this test binary
	wrapped.Env = cmd.Env
	return wrapped
}

// survives requires s still running a second from now.
func survives(t *gotest.T, s *started, msg string) {
	gotest.Consistently(t, time.Second, 50*time.Millisecond, func(poll *gotest.R) {
		select {
		case <-s.done:
			gotest.Fail(poll, msg)
		default:
		}
	})
}

// DrillLoopTestSuite: the drill's loop with real supervisors and synthetic
// workers. One worker takes down its supervisor or its own group, or
// signals the drill, mid-mutant; the others are still judged.
type DrillLoopTestSuite struct{}

func (s *DrillLoopTestSuite) SuiteGuard() string {
	if runtime.GOOS != "linux" {
		return "the drill runs on Linux only"
	}
	return ""
}

func (s *DrillLoopTestSuite) SuiteConfig() gotest.SuiteConfig {
	cfg := gotest.IntegrationSuiteConfig()
	cfg.Parallel = true
	return cfg
}

func (s *DrillLoopTestSuite) BeforeEach(t *gotest.T) *procs { return newProcs(t) }

func (s *DrillLoopTestSuite) AfterEach(t *gotest.T, p *procs) { p.stop() }

// loopReport is what the loop child prints last.
type loopReport struct {
	Stopped bool            `json:"stopped"`
	Caught  map[string]bool `json:"caught"`
}

// loopRun runs the loop over phases, each [names, parallel, cpus]; it sends
// the loop outside, if any, once a worker published its pid.
func loopRun(t *gotest.T, p *procs, phases string, outside syscall.Signal) (loopReport, string) {
	cmd := childCommand("loop", p.dir, phases)
	var stdout, stderr bytes.Buffer
	cmd.Stdout, cmd.Stderr = &stdout, &stderr
	l := p.start(t, cmd)
	if outside != 0 {
		p.pid(t, filepath.Join(p.dir, "leftover"))
		gotest.NoError(t, cmd.Process.Signal(outside))
	}
	code := l.wait(t, 2*time.Minute)
	gotest.Zero(t, code, "the loop failed:\n%s\n%s", stdout.String(), stderr.String())
	out := strings.TrimSpace(stdout.String())
	var report loopReport
	gotest.NoError(t, json.Unmarshal([]byte(out[strings.LastIndexByte(out, '\n')+1:]), &report))
	return report, out
}

func (s *DrillLoopTestSuite) TestASignalFromOutsideStillStopsTheDrill(t *gotest.T, p *procs) {
	report, out := loopRun(t, p, `[[["g", "a"], 1, 1]]`, syscall.SIGTERM)
	gotest.True(t, report.Stopped)
	gotest.Equal(t, map[string]bool{"g": false}, report.Caught)
	gotest.Contains(t, out, "DRILL  stopped by SIGTERM from pid "+strconv.Itoa(os.Getpid())+" (")
}

func (s *DrillLoopTestSuite) TestAHangupStopsTheDrillLikeATermination(t *gotest.T, p *procs) {
	report, out := loopRun(t, p, `[[["g", "a"], 1, 1]]`, syscall.SIGHUP)
	gotest.True(t, report.Stopped)
	gotest.Equal(t, map[string]bool{"g": false}, report.Caught)
	gotest.Contains(t, out, "DRILL  stopped by SIGHUP from pid "+strconv.Itoa(os.Getpid())+" (")
}

func (s *DrillLoopTestSuite) TestAHangupIgnoredAtStartDoesNotStopTheDrill(t *gotest.T, p *procs) {
	cmd := underNohup(t, childCommand("loop", p.dir, `[[["g", "a"], 1, 1]]`))
	var stdout, stderr bytes.Buffer
	cmd.Stdout, cmd.Stderr = &stdout, &stderr
	l := p.start(t, cmd)
	p.pid(t, filepath.Join(p.dir, "leftover"))
	gotest.NoError(t, cmd.Process.Signal(syscall.SIGHUP))
	survives(t, l, "a drill started under nohup stopped on SIGHUP")
	gotest.NoError(t, cmd.Process.Signal(syscall.SIGTERM))
	gotest.Zero(t, l.wait(t, 2*time.Minute), "the loop failed:\n%s\n%s", stdout.String(), stderr.String())
	gotest.NotContains(t, stdout.String(), "SIGHUP")
	gotest.Contains(t, stdout.String(), "DRILL  stopped by SIGTERM from pid "+strconv.Itoa(os.Getpid())+" (")
}

func (s *DrillLoopTestSuite) TestADeadWorkerFailsOnlyItsOwnMutant(t *gotest.T, p *procs) {
	report, out := loopRun(t, p, `[[["a", "b", "c", "d", "e"], 2, 1], [["f"], 1, 1]]`, 0)
	gotest.Regexp(t, `drill  ignored SIGTERM from pid \d+, a process under judgment`, out)
	gotest.Equal(t, map[string]bool{"a": true, "b": false, "c": false, "d": true, "e": true, "f": true}, report.Caught)
	gotest.False(t, report.Stopped)
	gotest.True(t, gone(p.pid(t, filepath.Join(p.dir, "leftover"))), "the dead supervisor's worker outlived the drill")
}

func (s *DrillLoopTestSuite) TestAStopAsAMutantStartsLeavesItWithoutAVerdict(t *gotest.T, p *procs) {
	results, err := judgeStoppedAtStart(p.dir)
	gotest.NoError(t, err)
	gotest.Empty(t, results)
}
