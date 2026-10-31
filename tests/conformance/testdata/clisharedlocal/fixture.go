package clisharedlocal

import "context"

// QueueSharedFixture declares an AfterEach.
type QueueSharedFixture struct {
	Addr string
}

func (f *QueueSharedFixture) BeforeAll(ctx context.Context) error { f.Addr = "queue"; return nil }

func (f *QueueSharedFixture) AfterEach(ctx context.Context) error { return nil }
