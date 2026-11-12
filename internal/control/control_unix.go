//go:build unix && !aix

package control

import (
	"os"
	"os/exec"
	"strconv"
	"sync"
	"syscall"
)

// movedFD is the lowest descriptor Claim moves the channel to, out of the
// way of descriptors a test or a library expects at the bottom.
const movedFD = 1000

// ChildEnd is the end a suite process inherits.
type ChildEnd struct {
	f    *os.File
	once sync.Once
}

// New creates the channel for one suite process. The runner's end is
// pollable, so closing it ends a read under way. Both ends are close-on-exec:
// only the command the child's end is attached to gets it.
func New() (*Link, *ChildEnd, error) {
	fds, err := socketpair()
	if err != nil {
		return nil, nil, os.NewSyscallError("socketpair", err)
	}
	if err := syscall.SetNonblock(fds[0], true); err != nil {
		_ = syscall.Close(fds[0])
		_ = syscall.Close(fds[1])
		return nil, nil, os.NewSyscallError("setnonblock", err)
	}
	runner := os.NewFile(uintptr(fds[0]), "gotest-control")
	return newLink(runner, runner), &ChildEnd{f: os.NewFile(uintptr(fds[1]), "gotest-control-child")}, nil
}

// Attach passes the end to cmd as its next extra file, which os/exec places
// at descriptor 3 plus its index, and names that descriptor in cmd's
// environment.
func (c *ChildEnd) Attach(cmd *exec.Cmd) {
	fd := 3 + len(cmd.ExtraFiles)
	cmd.ExtraFiles = append(cmd.ExtraFiles, c.f)
	setEnv(cmd, strconv.Itoa(fd))
}

// Close closes the end in the runner, once the command has started.
func (c *ChildEnd) Close() {
	c.once.Do(func() { _ = c.f.Close() })
}

// claim takes the socket at descriptor v. Standard streams and anything but
// a socket are left alone.
func claim(v string) *Endpoint {
	fd, ok := decimal(v)
	if !ok || fd < 3 {
		return nil
	}
	var st syscall.Stat_t
	if syscall.Fstat(fd, &st) != nil || st.Mode&syscall.S_IFMT != syscall.S_IFSOCK {
		return nil
	}
	if moved, _, errno := syscall.Syscall(syscall.SYS_FCNTL, uintptr(fd), syscall.F_DUPFD_CLOEXEC, movedFD); errno == 0 {
		_ = syscall.Close(fd)
		fd = int(moved)
	} else {
		// A descriptor limit at or below movedFD leaves the channel in place,
		// and so does OpenBSD, where syscall.Syscall takes no fcntl.
		syscall.CloseOnExec(fd)
	}
	if err := syscall.SetNonblock(fd, true); err != nil {
		_ = syscall.Close(fd)
		return nil
	}
	f := os.NewFile(uintptr(fd), "gotest-control")
	return newEndpoint(f, f)
}

// readNow reads what is buffered without waiting: a read with a deadline
// already past would not read at all.
func readNow(f *os.File, p []byte) (n int, eof bool) {
	rc, err := f.SyscallConn()
	if err != nil {
		return 0, true
	}
	var rerr error
	if err := rc.Read(func(fd uintptr) bool {
		for {
			n, rerr = syscall.Read(int(fd), p)
			if rerr != syscall.EINTR {
				return true
			}
		}
	}); err != nil {
		return 0, true
	}
	switch {
	case rerr == syscall.EAGAIN:
		return 0, false
	case rerr != nil, n <= 0:
		return 0, true
	}
	return n, false
}
