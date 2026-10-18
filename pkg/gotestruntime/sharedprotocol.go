package gotestruntime

import (
	"os"
	"strconv"

	"github.com/mvrahden/go-test/internal/protocol"
)

// ClaimSharedProtocol returns the stream a shared fixture process writes its
// protocol lines to and takes stdout away from user code first: whatever a
// fixture prints lands on stderr, where the runner passes it on, never on the
// protocol. The runner names the descriptor it reads; without one the
// process's own stdout is the stream.
func ClaimSharedProtocol() *os.File {
	stream := os.Stdout
	if fd, err := strconv.Atoi(os.Getenv(protocol.EnvSharedProtocolFD)); err == nil && fd > 2 {
		stream = os.NewFile(uintptr(fd), "shared-protocol")
	}
	os.Stdout = os.Stderr
	return stream
}
