package lintdeclembed

import (
	"context"

	"github.com/mvrahden/go-test/pkg/gotest"
)

type StoreFixture struct{ items []string }

func (f *StoreFixture) BeforeAll(context.Context) error { return nil }

type ChildTestSuite struct {
	*StoreFixture
}

func (s *ChildTestSuite) TestStores(t *gotest.T) { gotest.Empty(t, s.items) }
