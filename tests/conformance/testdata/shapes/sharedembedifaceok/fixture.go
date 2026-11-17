package sharedembedifaceok

import (
	"context"
	"sync"
)

// Clock is a handle the suites call.
type Clock interface{ Now() int64 }

type fixed int64

func (f fixed) Now() int64 { return int64(f) }

// Endpoint is a value the suites read.
type Endpoint string

type marker struct{}

// HandleSharedFixture embeds an interface and a value Hydrate assigns, and
// two structs with nothing to transfer.
type HandleSharedFixture struct {
	sync.Mutex
	marker
	Clock
	Endpoint
	Addr string
}

func (f *HandleSharedFixture) BeforeAll(ctx context.Context) error {
	f.Addr = "http://x"
	return nil
}

func (f *HandleSharedFixture) Hydrate(ctx context.Context) error {
	f.Clock = fixed(7)
	f.Endpoint = Endpoint(f.Addr)
	return nil
}

func (f *HandleSharedFixture) Dehydrate(ctx context.Context) error { return nil }
