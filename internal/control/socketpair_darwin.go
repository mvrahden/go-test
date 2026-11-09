package control

import "syscall"

// socketpair creates a connected pair, both ends close-on-exec. Darwin has no
// flag for that at creation; the fork lock keeps os/exec from starting a
// process in between.
func socketpair() ([2]int, error) {
	syscall.ForkLock.RLock()
	defer syscall.ForkLock.RUnlock()
	fds, err := syscall.Socketpair(syscall.AF_UNIX, syscall.SOCK_STREAM, 0)
	if err != nil {
		return fds, err
	}
	syscall.CloseOnExec(fds[0])
	syscall.CloseOnExec(fds[1])
	return fds, nil
}
