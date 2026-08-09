package testpkg

import (
	"fmt"
	"net"

	"github.com/mvrahden/go-test/pkg/gotest"
)

// GoroutineServiceTestSuite starts a goroutine that runs until something stops
// it — the shape every background server has — and stops it where this framework
// says resources get released, in AfterEach.
//
// The wait gotest.Go registers as cleanup runs after AfterEach, so the ordering
// works out on its own. Waiting inside the test instead would deadlock: the
// listener that ends the goroutine is not closed until AfterEach, which runs
// after the test's own defers.
type GoroutineServiceTestSuite struct {
	listener net.Listener
}

func (s *GoroutineServiceTestSuite) AfterEach(t *gotest.T) {
	if s.listener != nil {
		s.listener.Close()
	}
}

func (s *GoroutineServiceTestSuite) AfterAll(t *gotest.T) {
	fmt.Println("MARK:suite afterall")
}

func (s *GoroutineServiceTestSuite) TestServesUntilStopped(t *gotest.T) {
	l, err := net.Listen("tcp", "127.0.0.1:0")
	gotest.NoError(t, err)
	s.listener = l

	gotest.Go(t, func() {
		for {
			conn, err := l.Accept()
			if err != nil {
				return
			}
			conn.Close()
		}
	})

	conn, err := net.Dial("tcp", l.Addr().String())
	gotest.NoError(t, err)
	conn.Close()
	fmt.Println("MARK:served a connection")
}
