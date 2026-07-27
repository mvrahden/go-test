package testpkg

import (
	"sync"

	"github.com/mvrahden/go-test/pkg/gotest"
)

type Guarded struct {
	Name string
	mu   sync.Mutex
}

type UnexportedFuzzTestSuite struct{}

func (s *UnexportedFuzzTestSuite) TestOne(t *gotest.T) {}

func (s *UnexportedFuzzTestSuite) FuzzGuarded(f *gotest.F) {
	gotest.Fuzz(f, func(t *gotest.T, g Guarded) {
		gotest.True(t, g.Name == g.Name)
	})
}
