package drill

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"
	"unsafe"

	"golang.org/x/sys/unix"
)

// errCancelled refuses to start a command once the drill is stopping.
var errCancelled = errors.New("drill cancelled")

// Supervision runs commands under a supervisor and cancels every one still
// running when the drill stops.
type Supervision struct {
	supervisor []string
	seconds    float64
	logs       string
	pass       []*os.File

	mu       sync.Mutex
	active   map[*exec.Cmd]bool
	stopping bool
}

// NewSupervision supervises with the command prefix supervisor, which
// takes --supervise <seconds> <status> <command...>.
func NewSupervision(supervisor []string, seconds float64, logs string) *Supervision {
	return &Supervision{supervisor: supervisor, seconds: seconds, logs: logs, active: map[*exec.Cmd]bool{}}
}

// Pass hands files to every supervisor started from now on, as fds 3 and
// up. They are not close-on-exec there, so its command and everything that
// command starts inherit them too.
func (s *Supervision) Pass(files ...*os.File) { s.pass = append(s.pass, files...) }

// Stopping reports whether the drill was stopped.
func (s *Supervision) Stopping() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.stopping
}

// Stop cancels the drill: every running supervisor is told to stop.
func (s *Supervision) Stop() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.stopping = true
	for cmd := range s.active {
		_ = cmd.Process.Signal(syscall.SIGTERM)
	}
}

// sweep kills what a dead supervisor left: whatever this process adopted
// outside the trees of the supervisors still running.
func (s *Supervision) sweep() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	spare := map[int]bool{}
	for cmd := range s.active {
		spare[cmd.Process.Pid] = true
	}
	return Clean(spare)
}

// Run runs command under a supervisor and returns the supervisor's exit
// code and status, nil when it left none.
func (s *Supervision) Run(command []string, label string, stdout, stderr *os.File, dir string) (int, *Status, error) {
	status := filepath.Join(s.logs, label+".supervisor.json")
	if err := os.Remove(status); err != nil && !errors.Is(err, os.ErrNotExist) {
		return 0, nil, err
	}
	args := append(append(s.supervisor[1:len(s.supervisor):len(s.supervisor)], "--supervise",
		strconv.FormatFloat(s.seconds, 'g', -1, 64), status), command...)
	cmd := exec.Command(s.supervisor[0], args...) //nolint:gosec // G204: the drill's own supervisor
	cmd.Dir, cmd.Stdin, cmd.Stdout, cmd.Stderr = dir, os.Stdin, stdout, stderr
	cmd.ExtraFiles = s.pass
	// A session of its own: a signal to a supervisor's group reaches
	// neither the drill nor what started it.
	cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true}
	s.mu.Lock()
	if s.stopping {
		s.mu.Unlock()
		return 0, nil, errCancelled
	}
	if err := cmd.Start(); err != nil {
		s.mu.Unlock()
		return 0, nil, err
	}
	s.active[cmd] = true
	s.mu.Unlock()
	_ = cmd.Wait()
	s.mu.Lock()
	delete(s.active, cmd)
	s.mu.Unlock()
	code := exitCode(cmd.ProcessState)
	data, err := os.ReadFile(status)
	if err != nil {
		return code, nil, nil
	}
	var observed Status
	if json.Unmarshal(data, &observed) != nil {
		return code, nil, nil
	}
	return code, &observed, nil
}

// Phase is one round of judging: its mutants, how many run at once, the
// CPUs each gets, and each mutant's expected seconds; nil weighs all alike.
type Phase struct {
	Names    []string
	Parallel int
	CPUs     int
	Weight   func(string) float64
}

// UnmarshalJSON reads a phase as [names, parallel, cpus].
func (p *Phase) UnmarshalJSON(data []byte) error {
	var fields []json.RawMessage
	if err := json.Unmarshal(data, &fields); err != nil {
		return err
	}
	if len(fields) != 3 {
		return errors.New("a phase is [names, parallel, cpus]")
	}
	for i, target := range []any{&p.Names, &p.Parallel, &p.CPUs} {
		if err := json.Unmarshal(fields[i], target); err != nil {
			return err
		}
	}
	return nil
}

// Execute judges one mutant on a worker slot with a CPU budget and returns
// its verdict and its supervisor's status.
type Execute func(worker int, name string, cpus int) (Verdict, *Status, error)

// JudgeAll judges each phase's mutants on its workers, one phase after the
// other, recording every verdict in results.
func JudgeAll(phases []Phase, s *Supervision, execute Execute, results map[string]Verdict) error {
	record := func(name string, verdict Verdict, observed *Status) error {
		if observed == nil || observed.CleanupError != "" {
			// A supervisor that died or could not clean up fails its own
			// mutant; what it left behind is killed before the worker slot
			// takes the next. Should that fail, nothing more is judged.
			if err := s.sweep(); errors.Is(err, errSurvived) {
				s.Stop()
			} else if err != nil {
				return err
			}
		}
		s.mu.Lock()
		defer s.mu.Unlock()
		results[name] = verdict
		lead := "DRILL  "
		if verdict.Caught {
			lead = "drill  "
		}
		fmt.Printf("%s%s: %s (%.1fs)\n", lead, name, verdict.Reason, verdict.Seconds)
		return nil
	}
	for _, phase := range phases {
		queues := NewQueues(phase.Names, phase.Parallel, phase.Weight)
		errs := make([]chan error, phase.Parallel)
		for i := range errs {
			errs[i] = make(chan error, 1)
			go func(worker int) {
				errs[worker] <- func() error {
					defer queues.Done(worker)
					for !s.Stopping() {
						name, ok := queues.Next(worker)
						if !ok {
							return nil
						}
						verdict, observed, err := execute(worker, name, phase.CPUs)
						if errors.Is(err, errCancelled) {
							// Stopped meanwhile: the mutant has no verdict.
							return nil
						}
						if err != nil {
							return err
						}
						if err := record(name, verdict, observed); err != nil {
							return err
						}
					}
					return nil
				}()
			}(i)
		}
		// A worker's error fails the drill, as soon as it is the earliest
		// worker still unaccounted for to report one.
		var failure error
		for _, ch := range errs {
			if err := <-ch; err != nil && failure == nil {
				failure = err
				s.Stop()
			}
		}
		if failure != nil {
			return failure
		}
	}
	return nil
}

// sender names a signal's sender for the log, as far as it is still there
// to read.
func sender(pid int) string {
	if pid == 0 {
		return "the kernel or a terminal"
	}
	data, err := os.ReadFile("/proc/" + strconv.Itoa(pid) + "/cmdline")
	command := strings.TrimSpace(strings.ToValidUTF8(strings.ReplaceAll(string(data), "\x00", " "), "�"))
	if err != nil || command == "" {
		return "pid " + strconv.Itoa(pid) + ", already gone"
	}
	return "pid " + strconv.Itoa(pid) + " (" + command + ")"
}

// stopSignals stop a drill or a supervisor. SIGHUP counts only when not
// ignored at start, as under nohup: Notify would un-ignore it. Read at init,
// before any Notify.
var stopSignals = func() []os.Signal {
	sigs := []os.Signal{syscall.SIGINT, syscall.SIGTERM}
	if !signal.Ignored(syscall.SIGHUP) {
		sigs = append(sigs, syscall.SIGHUP)
	}
	return sigs
}()

// siginfo is the head of the kernel's siginfo_t for a signal sent by kill.
type siginfo struct {
	Signo int32
	Errno int32
	Code  int32
	_     [unsafe.Sizeof(uintptr(0)) - 4]byte
	Pid   int32
	UID   uint32
	_     [112]byte
}

// WatchSignals runs work while the calling goroutine, which must be locked
// to the main thread, cancels s on a stop signal from outside
// the drill.
// One sent by a process the drill started is a mutant's doing, so it is
// reported and ignored.
//
// Only sigtimedwait names a signal's sender. The main thread keeps the
// signals blocked and waits for them; the kernel hands a process-directed
// signal to the main thread first whenever it waits. One that reaches
// another thread anyway carries no sender and stops the drill.
func WatchSignals(s *Supervision, work func()) {
	fallback := make(chan os.Signal, 4)
	signal.Notify(fallback, stopSignals...)
	defer signal.Stop(fallback)
	var set, previous unix.Sigset_t
	for _, sig := range stopSignals {
		sig := sig.(syscall.Signal)
		set.Val[(sig-1)/64] |= 1 << ((uint(sig) - 1) % 64)
	}
	if err := unix.PthreadSigmask(unix.SIG_BLOCK, &set, &previous); err != nil {
		panic(err)
	}
	defer func() { _ = unix.PthreadSigmask(unix.SIG_SETMASK, &previous, nil) }()
	done := make(chan struct{})
	go func() {
		defer close(done)
		work()
	}()
	handle := func(sig syscall.Signal, pid int) {
		name := signalName(int(sig))
		if pid != 0 {
			if _, ours := Descendants(nil)[pid]; ours {
				fmt.Printf("drill  ignored %s from pid %d, a process under judgment\n", name, pid)
				return
			}
		}
		fmt.Printf("DRILL  stopped by %s from %s\n", name, sender(pid))
		s.Stop()
	}
	timeout := unix.NsecToTimespec((100 * time.Millisecond).Nanoseconds())
	for {
		select {
		case <-done:
			return
		case sig := <-fallback:
			handle(sig.(syscall.Signal), 0)
			continue
		default:
		}
		var info siginfo
		r, _, errno := unix.Syscall6(unix.SYS_RT_SIGTIMEDWAIT, uintptr(unsafe.Pointer(&set)),
			uintptr(unsafe.Pointer(&info)), uintptr(unsafe.Pointer(&timeout)), 8, 0, 0)
		if errno != 0 {
			continue
		}
		handle(syscall.Signal(r), int(info.Pid))
	}
}

// availableCPUs is the CPU affinity, capped by a cgroup CPU quota:
// containers can expose a large mask but a much smaller quota.
func availableCPUs() int {
	var set unix.CPUSet
	cpus := 1
	if unix.SchedGetaffinity(0, &set) == nil {
		cpus = set.Count()
	}
	data, err := os.ReadFile("/sys/fs/cgroup/cpu.max")
	if err != nil {
		return cpus
	}
	fields := strings.Fields(string(data))
	if len(fields) != 2 || fields[0] == "max" {
		return cpus
	}
	quota, err1 := strconv.Atoi(fields[0])
	period, err2 := strconv.Atoi(fields[1])
	if err1 != nil || err2 != nil || period == 0 {
		return cpus
	}
	return min(cpus, max(1, quota/period))
}
