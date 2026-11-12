package sharedembedhyd

import "context"

// Info is rebuilt field by field in every process.
type Info struct {
	Label string
	Host  string
}

// StateSharedFixture embeds Info and Hydrate assigns each field it promotes.
type StateSharedFixture struct {
	Info
	Addr string
}

func (f *StateSharedFixture) BeforeAll(ctx context.Context) error {
	f.Addr = "mem://state"
	return nil
}

func (f *StateSharedFixture) Hydrate(ctx context.Context) error {
	f.Label = "from " + f.Addr
	f.Host = "mem"
	return nil
}

func (f *StateSharedFixture) Dehydrate(ctx context.Context) error { return nil }
