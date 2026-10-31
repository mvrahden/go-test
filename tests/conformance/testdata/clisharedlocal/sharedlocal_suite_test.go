package clisharedlocal

import "github.com/mvrahden/go-test/pkg/gotest"

type SharedLocalTestSuite struct {
	Queue *QueueSharedFixture
}

func (s *SharedLocalTestSuite) TestBody(t *gotest.T) {}
