package lintdeclhookcall

import (
	"context"

	"github.com/mvrahden/go-test/pkg/gotest"
)

type StoreFixture struct{ resets int }

func (f *StoreFixture) BeforeAll(context.Context) error { return nil }

func (f *StoreFixture) BeforeEach(context.Context) error {
	f.resets++
	return nil
}

type HookTestSuite struct {
	Store *StoreFixture
}

func (s *HookTestSuite) BeforeEach(t *gotest.T) {
	gotest.NoError(t, s.Store.BeforeEach(t.Context()))
}

func (s *HookTestSuite) TestResets(t *gotest.T) { gotest.Equal(t, 2, s.Store.resets) }
