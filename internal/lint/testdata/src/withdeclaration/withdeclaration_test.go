package withdeclaration

import (
	"context"

	"github.com/mvrahden/go-test/pkg/gotest"
)

type (
	helper struct{ n int }

	// GroupedTestSuite is declared inside a type block.
	GroupedTestSuite struct { // want `suite GroupedTestSuite is declared inside a type block; give it its own type declaration`
		Store *StoreFixture
		h     helper
	}
)

func (s *GroupedTestSuite) TestGrouped(t *gotest.T) {}

type (
	// StoreFixture is bound by GroupedTestSuite.
	StoreFixture struct{ n int } // want `fixture StoreFixture is declared inside a type block; give it its own type declaration`
	counter      int
)

func (f *StoreFixture) BeforeAll(context.Context) error { return nil }

// A type named like a fixture that no suite binds is an ordinary type.
type (
	LooseFixture struct{}
	more         int
)

// A block holding one spec is a declaration of its own.
type (
	SoloTestSuite struct{}
)

func (s *SoloTestSuite) TestSolo(t *gotest.T) {}

type (
	plainA int
	plainB int
)
