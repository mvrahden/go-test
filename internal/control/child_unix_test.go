//go:build unix && !aix

package control_test

import (
	"fmt"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"syscall"

	"golang.org/x/sys/unix"

	"github.com/mvrahden/go-test/internal/control"
	"github.com/mvrahden/go-test/pkg/gotest"
)

var spawnWays = []way{
	{"exec", func(env []string) error {
		cmd := exec.Command(os.Args[0]) //nolint:gosec // G204: this test binary
		cmd.Env = env
		return cmd.Start()
	}},
	{"shell", func(env []string) error {
		cmd := exec.Command("/bin/sh", "-c", `"$0"; exit $?`, os.Args[0]) //nolint:gosec // G204: this test binary
		cmd.Env = env
		return cmd.Start()
	}},
}

func fstat(fd int) (syscall.Stat_t, error) {
	var st syscall.Stat_t
	err := syscall.Fstat(fd, &st)
	return st, err
}

// identity is what tells the channel's socket apart at fd: its inode.
func identity(fd uintptr) (string, bool) {
	st, err := fstat(int(fd)) //nolint:gosec // G115: a descriptor number
	if err != nil || st.Mode&syscall.S_IFMT != syscall.S_IFSOCK {
		return "", false
	}
	return strconv.FormatUint(st.Ino, 10), true
}

func identify(raw string) []string {
	fd, err := strconv.Atoi(raw)
	if err != nil {
		return nil
	}
	id, _ := identity(uintptr(fd))
	return []string{id}
}

// reportClaimed reports where the channel went and returns what identifies
// it at both its old and its new descriptor.
func reportClaimed(e *control.Endpoint, raw string, ids []string) string {
	moved, _ := e.Handles()
	switch id, ok := identity(3); {
	case ok && len(ids) == 1 && id == ids[0]:
		report("fd3", "channel")
	case ok:
		report("fd3", "socket")
	default:
		report("fd3", "not-a-socket")
	}
	report("moved", moved)
	flags, err := unix.FcntlInt(moved, unix.F_GETFD, 0)
	report("cloexec", err == nil && flags&unix.FD_CLOEXEC != 0)
	if len(ids) != 1 {
		return ""
	}
	return fmt.Sprintf("%s=%s,%d=%s", raw, ids[0], moved, ids[0])
}

// held lists the descriptors of expect that hold the channel here.
func held(expect string) string {
	var fds []string
	for pair := range strings.SplitSeq(expect, ",") {
		h, id, _ := strings.Cut(pair, "=")
		fd, err := strconv.Atoi(h)
		if err != nil {
			continue
		}
		if got, ok := identity(uintptr(fd)); ok && got == id {
			fds = append(fds, h)
		}
	}
	if len(fds) == 0 {
		return "none"
	}
	return strings.Join(fds, ",")
}

// untouched reports whether the descriptor raw names is still a regular file.
func untouched(raw string) bool {
	fd, err := strconv.Atoi(raw)
	if err != nil {
		return false
	}
	st, err := fstat(fd)
	return err == nil && st.Mode&syscall.S_IFMT == syscall.S_IFREG
}

func standardValue() string { return "1" }

// attachFile passes f as the child's descriptor 3 and returns the value
// naming it.
func attachFile(cmd *exec.Cmd, f *os.File) string {
	cmd.ExtraFiles = append(cmd.ExtraFiles, f)
	return "3"
}

// socketStdout makes the child's stdout one end of a socket pair, so a
// standard stream passes the channel's type check, and returns the other end.
func socketStdout(cmd *exec.Cmd) (r, w *os.File, err error) {
	fds, err := syscall.Socketpair(syscall.AF_UNIX, syscall.SOCK_STREAM, 0)
	if err != nil {
		return nil, nil, err
	}
	syscall.CloseOnExec(fds[0])
	syscall.CloseOnExec(fds[1])
	r, w = os.NewFile(uintptr(fds[0]), "stdout-r"), os.NewFile(uintptr(fds[1]), "stdout-w")
	cmd.Stdout = w
	return r, w, nil
}

// prepareAttach gives cmd an extra file of its own before the channel's.
func prepareAttach(cmd *exec.Cmd) { cmd.ExtraFiles = []*os.File{os.Stdin} }

// checkAttached checks that the channel follows cmd's own extra files.
func checkAttached(t *gotest.T, cmd *exec.Cmd, entry string, prepared bool) {
	want := 3
	if prepared {
		want = 4
	}
	gotest.Len(t, cmd.ExtraFiles, want-2)
	gotest.Equal(t, control.Env+"="+strconv.Itoa(want), entry)
}

// checkClaimed checks that the channel left descriptor 3 for one above the
// usual range, closed on exec.
func checkClaimed(t *gotest.T, c *child) {
	gotest.NotEqual(t, "channel", c.seen["fd3"])
	moved, err := strconv.Atoi(c.seen["moved"])
	gotest.NoError(t, err)
	gotest.GreaterOrEqual(t, moved, 1000)
	gotest.Equal(t, "true", c.seen["cloexec"])
}

// lowerLimit sets the soft descriptor limit to n.
func lowerLimit(n string) error {
	var rl syscall.Rlimit
	if err := syscall.Getrlimit(syscall.RLIMIT_NOFILE, &rl); err != nil {
		return err
	}
	// The field's integer type differs between systems.
	if _, err := fmt.Sscan(n, &rl.Cur); err != nil {
		return err
	}
	return syscall.Setrlimit(syscall.RLIMIT_NOFILE, &rl)
}
