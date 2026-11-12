package cutshort_test

import (
	"time"

	"github.com/mvrahden/go-test/pkg/gotest"
)

type CutShortTestSuite struct{}

func (s *CutShortTestSuite) TestAFast(t *gotest.T) {
	gotest.True(t, true)
}

// TestBDies never returns: the panic is on a goroutine no test owns.
func (s *CutShortTestSuite) TestBDies(t *gotest.T) {
	go func() { panic("boom on a goroutine") }()
	time.Sleep(time.Minute)
}
