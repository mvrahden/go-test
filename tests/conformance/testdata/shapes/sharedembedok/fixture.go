package sharedembedok

import (
	"context"
	"sync"
)

// Info is transferred through a named field.
type Info struct{ Addr string }

// Derived is rebuilt in every process.
type Derived struct{ Label string }

// StateSharedFixture embeds a type without exported fields and one Hydrate assigns.
type StateSharedFixture struct {
	sync.Mutex
	Derived
	Info Info
}

func (f *StateSharedFixture) BeforeAll(ctx context.Context) error {
	f.Info = Info{Addr: "mem://state"}
	return nil
}

func (f *StateSharedFixture) Hydrate(ctx context.Context) error {
	f.Derived = Derived{Label: "from " + f.Info.Addr}
	return nil
}

func (f *StateSharedFixture) Dehydrate(ctx context.Context) error { return nil }
