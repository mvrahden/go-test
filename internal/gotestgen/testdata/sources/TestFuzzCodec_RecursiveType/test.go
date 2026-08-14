package testpkg

import "github.com/mvrahden/go-test/pkg/gotest"

type Node struct {
	Label string
	Next  *Node
}

type RecursiveFuzzTestSuite struct{}

func (s *RecursiveFuzzTestSuite) TestOne(t *gotest.T) {}

func (s *RecursiveFuzzTestSuite) FuzzNode(f *gotest.F) {
	gotest.Fuzz(f, func(t *gotest.T, n Node) {
		gotest.True(t, n.Label == n.Label)
	})
}
