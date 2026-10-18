//go:build darwin

package gotestruntime

import "syscall"

// stdoutPipe is the pipe fd 1 was when the process started under the runner,
// zero otherwise: a terminal, a file or a plain go test run never counts.
var stdoutPipe = sampleStdoutPipe()

type pipeID struct{ dev, ino uint64 }

func sampleStdoutPipe() pipeID {
	if stopFile == "" {
		return pipeID{}
	}
	var st syscall.Stat_t
	if syscall.Fstat(1, &st) != nil || st.Mode&syscall.S_IFMT != syscall.S_IFIFO {
		return pipeID{}
	}
	return pipeID{dev: uint64(st.Dev), ino: st.Ino} //nolint:gosec // G115: a device number is never negative
}

// stdoutReaderGone reports whether nobody reads the pipe fd 1 was at start
// any more. A converter that died leaves it so, even where the process's
// parent, an -exec wrapper, lives on. The write filter of a pipe whose
// reader closed reports EOF, or refuses with EPIPE.
func stdoutReaderGone() bool {
	if stdoutPipe == (pipeID{}) {
		return false
	}
	var st syscall.Stat_t
	if syscall.Fstat(1, &st) != nil || (pipeID{dev: uint64(st.Dev), ino: st.Ino}) != stdoutPipe { //nolint:gosec // G115: a device number is never negative
		return false // fd 1 was moved since: not the stream the runner reads
	}
	kq, err := syscall.Kqueue()
	if err != nil {
		return false
	}
	defer syscall.Close(kq) //nolint:errcheck // a kqueue of this call's own
	var ev syscall.Kevent_t
	syscall.SetKevent(&ev, 1, syscall.EVFILT_WRITE, syscall.EV_ADD)
	var out [1]syscall.Kevent_t
	n, err := syscall.Kevent(kq, []syscall.Kevent_t{ev}, out[:], &syscall.Timespec{})
	switch {
	case err == syscall.EPIPE:
		return true
	case err != nil || n != 1:
		return false
	case out[0].Flags&syscall.EV_ERROR != 0:
		return out[0].Data == int64(syscall.EPIPE)
	}
	return out[0].Flags&syscall.EV_EOF != 0
}
