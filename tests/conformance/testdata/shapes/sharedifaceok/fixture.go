package sharedifaceok

import "context"

// Clock is an interface with a method.
type Clock interface{ Now() int64 }

type fixed int64

func (f fixed) Now() int64 { return int64(f) }

// Node refers to itself.
type Node struct {
	Name string
	Next *Node
}

// ChainSharedFixture assigns its interface field in Hydrate and transfers a
// field of a type that refers to itself.
type ChainSharedFixture struct {
	Clock Clock
	Chain *Node
	Any   any
}

func (f *ChainSharedFixture) BeforeAll(ctx context.Context) error {
	f.Chain = &Node{Name: "a", Next: &Node{Name: "b"}}
	f.Any = "free"
	return nil
}

func (f *ChainSharedFixture) Hydrate(ctx context.Context) error {
	f.Clock = fixed(7)
	return nil
}

func (f *ChainSharedFixture) Dehydrate(ctx context.Context) error { return nil }
