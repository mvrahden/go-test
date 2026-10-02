package fuzzreject

import "github.com/mvrahden/go-test/pkg/gotest"

type Frame struct {
	Done chan struct{}
}

type FrameTestSuite struct{}

func (s *FrameTestSuite) TestSomething(t *gotest.T) {}

func (s *FrameTestSuite) FuzzFrame(f *gotest.F) {
	f.Fuzz(func(t *gotest.T, in Frame) {
		gotest.NotNil(t, in.Done)
	})
}
