package sharedembedpart

import "context"

// Info has fields Hydrate leaves unassigned.
type Info struct {
	Label string
	Host  string
	Port  int
}

// StateSharedFixture embeds Info and Hydrate assigns one of its fields.
type StateSharedFixture struct {
	Info
	Addr string
}

func (f *StateSharedFixture) BeforeAll(ctx context.Context) error {
	f.Addr = "mem://state"
	f.Host = "mem"
	return nil
}

func (f *StateSharedFixture) Hydrate(ctx context.Context) error {
	f.Label = "from " + f.Addr
	return nil
}

func (f *StateSharedFixture) Dehydrate(ctx context.Context) error { return nil }
