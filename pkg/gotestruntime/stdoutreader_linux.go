//go:build linux

package gotestruntime

import (
	"syscall"
	"unsafe"
)

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
	return pipeID{dev: st.Dev, ino: st.Ino}
}

func watchesStdout() bool { return stdoutPipe != (pipeID{}) }

// stdoutReaderGone reports whether nobody reads the pipe fd 1 was at start
// any more. A converter that died leaves it so, even where the process's
// parent, an -exec wrapper, lives on and no death signal comes.
func stdoutReaderGone() bool {
	if stdoutPipe == (pipeID{}) {
		return false
	}
	var st syscall.Stat_t
	if syscall.Fstat(1, &st) != nil || (pipeID{dev: st.Dev, ino: st.Ino}) != stdoutPipe {
		return false // fd 1 was moved since: not the stream the runner reads
	}
	const pollErr, pollHup = 0x8, 0x10
	fds := [1]struct {
		fd             int32
		events, revent int16
	}{{fd: 1}}
	var now syscall.Timespec
	n, _, errno := syscall.Syscall6(syscall.SYS_PPOLL, uintptr(unsafe.Pointer(&fds[0])), 1, uintptr(unsafe.Pointer(&now)), 0, 0, 0)
	return errno == 0 && n == 1 && fds[0].revent&(pollErr|pollHup) != 0
}
