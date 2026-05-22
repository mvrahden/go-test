package testpkg

import (
	"context"
	"github.com/mvrahden/go-test/pkg/gotest"
)

type UsedFixture struct{}
func (f *UsedFixture) BeforeAll(ctx context.Context) error { return nil }

type UnusedFixture struct{}
func (f *UnusedFixture) BeforeAll(ctx context.Context) error { return nil }

type MyTestSuite struct { *UsedFixture }
func (s *MyTestSuite) TestOne(t *gotest.T) {}
