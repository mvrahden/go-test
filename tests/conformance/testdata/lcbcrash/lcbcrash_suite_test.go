// Package lcbcrash has a suite whose process dies on its own and a suite
// that hangs until the run's --timeout ends it.
package lcbcrash

import (
	"time"

	"github.com/mvrahden/go-test/pkg/gotest"
)

// CrashTestSuite's process dies under its method at once: a panic on a
// goroutine nobody recovers.
type CrashTestSuite struct{}

func (s *CrashTestSuite) TestCrash(t *gotest.T) {
	go func() { panic("the suite process crashes") }()
	select {}
}

// HangTestSuite's method hangs and ignores its context, so the deadline
// ends the process under it.
type HangTestSuite struct{}

func (s *HangTestSuite) TestHang(t *gotest.T) { time.Sleep(time.Hour) }
