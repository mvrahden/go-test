package clisharedlocal

import (
	"context"

	"github.com/mvrahden/go-test/pkg/gotest"
)

// QueueSharedFixture declares an AfterEach.
type QueueSharedFixture struct {
	Addr string
}

func (f *QueueSharedFixture) BeforeAll(ctx context.Context) error { f.Addr = "queue"; return nil }

func (f *QueueSharedFixture) AfterEach(ctx context.Context) error { return nil }

type SharedLocalTestSuite struct {
	Queue *QueueSharedFixture
}

func (s *SharedLocalTestSuite) TestBody(t *gotest.T) {}
