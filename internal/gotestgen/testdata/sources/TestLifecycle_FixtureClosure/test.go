package testpkg

import (
	"context"
	"fmt"

	"github.com/mvrahden/go-test/pkg/gotest"
)

type MarkSharedFixture struct{ Addr string }

func (f *MarkSharedFixture) BeforeAll(ctx context.Context) error { return nil }

type RootFixture struct{}

func (f *RootFixture) BeforeAll(ctx context.Context) error {
	fmt.Println("MARK:root setup")
	return nil
}

type ChildFixture struct{ Root *RootFixture }

func (f *ChildFixture) BeforeAll(ctx context.Context) error {
	fmt.Println("MARK:child setup")
	return nil
}

// UnusedFixture is bound by UnusedTestSuite only, so MixedTestSuite must never
// set it up.
type UnusedFixture struct{}

func (f *UnusedFixture) BeforeAll(ctx context.Context) error {
	fmt.Println("MARK:unused setup")
	return nil
}

// MixedTestSuite binds a package fixture, through it a parent, and a shared
// fixture, in each wrapper kind.
type MixedTestSuite struct {
	Child  *ChildFixture
	Shared *MarkSharedFixture
}

func (s *MixedTestSuite) wired(t interface {
	Errorf(string, ...any)
	FailNow()
}) {
	gotest.NotZero(t, s.Child, "package fixture field")
	gotest.NotZero(t, s.Child.Root, "parent fixture field")
	gotest.NotZero(t, s.Shared, "shared fixture field")
}

func (s *MixedTestSuite) TestWired(t *gotest.T) { s.wired(t) }

func (s *MixedTestSuite) BenchmarkWired(b *gotest.B) {
	s.wired(b)
	for b.Loop() {
	}
}

func (s *MixedTestSuite) FuzzWired(f *gotest.F) {
	f.Add("seed")
	f.Fuzz(func(t *gotest.T, in string) {
		s.wired(t)
		gotest.Equal(t, in, string([]byte(in)))
	})
}

type UnusedTestSuite struct{ Unused *UnusedFixture }

func (s *UnusedTestSuite) TestOne(t *gotest.T) { gotest.NotZero(t, s.Unused) }
