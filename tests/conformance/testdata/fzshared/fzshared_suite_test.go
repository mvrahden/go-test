package fzshared

import (
	"sync"

	"github.com/mvrahden/go-test/pkg/gotest"
)

// SharedTestSuite reads its shared fixture from the fuzz body.
type SharedTestSuite struct {
	Shared *StateSharedFixture
}

var once sync.Once

// FuzzRead marks, once per process, the state its body sees.
func (s *SharedTestSuite) FuzzRead(f *gotest.F) {
	f.Fuzz(func(t *gotest.T, in string) {
		once.Do(func() { mark("body saw " + s.Shared.State) })
	})
}

// FuzzCrash panics on its checked-in crasher, naming the state it sees.
func (s *SharedTestSuite) FuzzCrash(f *gotest.F) {
	f.Fuzz(func(t *gotest.T, in string) {
		if in == "boom" {
			panic("crashed on boom with the fixture " + s.Shared.State)
		}
	})
}
