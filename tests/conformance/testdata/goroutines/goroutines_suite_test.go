// Package goroutines starts goroutines with gotest.Go in the places that test
// where the wait on them ends.
package goroutines

import (
	"github.com/mvrahden/go-test/pkg/gotest"
)

// ServerTestSuite starts a goroutine in BeforeAll that only AfterAll stops.
type ServerTestSuite struct{ stop chan struct{} }

func (s *ServerTestSuite) BeforeAll(t *gotest.T) {
	s.stop = make(chan struct{})
	gotest.Go(t, func() { <-s.stop })
}

func (s *ServerTestSuite) AfterAll(t *gotest.T) { close(s.stop) }

func (s *ServerTestSuite) TestServed(t *gotest.T) {}

// GoexitTestSuite's goroutine fails fatally before the method waits on it.
type GoexitTestSuite struct{}

func (s *GoexitTestSuite) TestGoexit(t *gotest.T) {
	wait := gotest.Go(t, func() { gotest.Fail(t, "from the goroutine") })
	wait()
	gotest.Fail(t, "the method went on after the wait")
}
