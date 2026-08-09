package testpkg

import (
	"context"
	"fmt"

	"github.com/mvrahden/go-test/pkg/gotest"
)

type GoResourceFixture struct{}

func (f *GoResourceFixture) BeforeAll(_ context.Context) error { return nil }
func (f *GoResourceFixture) AfterAll(_ context.Context) error {
	fmt.Println("MARK:fixture released")
	return nil
}

// GoroutinePanicTestSuite panics on a goroutine the test started. Started with a
// bare `go`, that aborts the process with no cleanup at all; started with
// gotest.Go, it is carried back to the test's own goroutine.
type GoroutinePanicTestSuite struct {
	*GoResourceFixture
}

func (s *GoroutinePanicTestSuite) AfterAll(t *gotest.T) {
	fmt.Println("MARK:suite afterall")
}

func (s *GoroutinePanicTestSuite) TestGoroutinePanics(t *gotest.T) {
	wait := gotest.Go(t, func() { panic("boom on a spawned goroutine") })
	wait()
}
