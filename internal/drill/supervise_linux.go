package drill

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"os/signal"
	"strconv"
	"strings"
	"syscall"
	"time"

	"golang.org/x/sys/unix"
)

// errSurvived is Clean's failure: descendants outlived its deadline.
var errSurvived = errors.New("descendants survived cleanup")

type proc struct {
	birth  string
	parent int
}

// statFields reads a process's stat fields after its command name.
func statFields(pid int) ([]string, error) {
	data, err := os.ReadFile("/proc/" + strconv.Itoa(pid) + "/stat")
	if err != nil {
		return nil, err
	}
	s := string(data)
	i := strings.LastIndexByte(s, ')')
	if i < 0 {
		return nil, errors.New("malformed stat")
	}
	fields := strings.Fields(s[i+1:])
	if len(fields) < 20 {
		return nil, errors.New("malformed stat")
	}
	return fields, nil
}

// Descendants is every process below this one but the trees rooted at
// spare, by pid with its start time and parent.
func Descendants(spare map[int]bool) map[int]proc {
	entries, _ := os.ReadDir("/proc")
	parents, births := map[int]int{}, map[int]string{}
	for _, e := range entries {
		pid, err := strconv.Atoi(e.Name())
		if err != nil {
			continue
		}
		fields, err := statFields(pid)
		if err != nil {
			continue
		}
		parent, err := strconv.Atoi(fields[1])
		if err != nil {
			continue
		}
		parents[pid], births[pid] = parent, fields[19]
	}
	self := os.Getpid()
	owned := map[int]bool{self: true}
	for {
		grew := false
		for pid, parent := range parents {
			if owned[parent] && !owned[pid] && !spare[pid] {
				owned[pid], grew = true, true
			}
		}
		if !grew {
			break
		}
	}
	found := map[int]proc{}
	for pid := range owned {
		if pid != self {
			found[pid] = proc{births[pid], parents[pid]}
		}
	}
	return found
}

// Clean kills and reaps every descendant outside the trees rooted at spare.
// It kills before reaping, since a zombie's pid cannot be reused while it is
// ours, and repeats for descendants adopted meanwhile. Only killed children
// are reaped, so a spared child's exit status stays its waiter's.
func Clean(spare map[int]bool) error {
	deadline := time.Now().Add(5 * time.Second)
	self := os.Getpid()
	for {
		found := Descendants(spare)
		for pid, p := range found {
			if err := kill(pid, p.birth); err != nil {
				return err
			}
		}
		for pid, p := range found {
			if p.parent == self {
				var ws unix.WaitStatus
				_, _ = unix.Wait4(pid, &ws, unix.WNOHANG, nil)
			}
		}
		if len(Descendants(spare)) == 0 {
			return nil
		}
		if time.Now().After(deadline) {
			return errSurvived
		}
		time.Sleep(10 * time.Millisecond)
	}
}

// kill sends SIGKILL to pid if it is still the process born at birth. A
// pidfd pins identity; the start time is rechecked after opening it, since
// the pid may have been reaped and reused after the /proc scan.
func kill(pid int, birth string) error {
	fd, err := unix.PidfdOpen(pid, 0)
	if errors.Is(err, unix.ESRCH) {
		return nil
	}
	if err != nil {
		return err
	}
	defer func() { _ = unix.Close(fd) }()
	fields, err := statFields(pid)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	if fields[19] != birth {
		return nil
	}
	if err := unix.PidfdSendSignal(fd, unix.SIGKILL, nil, 0); err != nil && !errors.Is(err, unix.ESRCH) {
		return err
	}
	return nil
}

// EnableSubreaper makes this process collect every orphaned descendant. It
// refuses a kernel without pidfds before anything is started.
func EnableSubreaper() error {
	fd, err := unix.PidfdOpen(os.Getpid(), 0)
	if err != nil {
		return fmt.Errorf("pidfd_open: %w", err)
	}
	_ = unix.Close(fd)
	if err := unix.Prctl(unix.PR_SET_CHILD_SUBREAPER, 1, 0, 0, 0); err != nil {
		return fmt.Errorf("cannot become a subreaper: %w", err)
	}
	return nil
}

// supervise runs command in a session of its own, bounded by seconds and by
// a stop signal, then kills whatever it left. It writes what it observed
// to statusPath and returns nonzero when the bound or the cleanup failed.
func supervise(seconds float64, statusPath string, command []string) int {
	if err := EnableSubreaper(); err != nil {
		fmt.Fprintln(os.Stderr, "DRILL  "+err.Error())
		return 1
	}
	stopped := make(chan os.Signal, 2)
	signal.Notify(stopped, stopSignals...)
	began := time.Now()
	status := Status{}
	failed := false
	cmd := exec.Command(command[0], command[1:]...) //nolint:gosec // G204: the supervised command
	cmd.Stdin, cmd.Stdout, cmd.Stderr = os.Stdin, os.Stdout, os.Stderr
	cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true}
	if err := cmd.Start(); err != nil {
		fmt.Fprintln(os.Stderr, "DRILL  "+err.Error())
		failed = true
	} else {
		exited := make(chan *os.ProcessState, 1)
		go func() {
			_ = cmd.Wait()
			exited <- cmd.ProcessState
		}()
		deadline := time.NewTimer(time.Duration(seconds * float64(time.Second)))
		select {
		case ps := <-exited:
			if ps != nil {
				code := exitCode(ps)
				status.Exit = &code
			}
		case <-stopped:
			status.Interrupted = true
		case <-deadline.C:
			status.Timeout = true
		}
		deadline.Stop()
	}
	// Clean reaps after killing, the command itself included.
	if err := Clean(nil); err != nil {
		status.CleanupError = err.Error()
	}
	status.Seconds = time.Since(began).Seconds()
	if err := WriteJSONLine(statusPath, status); err != nil {
		fmt.Fprintln(os.Stderr, "DRILL  "+err.Error())
		return 1
	}
	if failed || status.Timeout || status.Interrupted || status.CleanupError != "" {
		return 1
	}
	return 0
}
