package testpkg

import (
	"context"
	"github.com/mvrahden/go-test/pkg/gotest"
)

type FullFixture struct{ Val string }
func (f *FullFixture) BeforeAll(ctx context.Context) error  { return nil }
func (f *FullFixture) AfterAll(ctx context.Context) error   { return nil }
func (f *FullFixture) BeforeEach(ctx context.Context) error { return nil }
func (f *FullFixture) AfterEach(ctx context.Context) error  { return nil }

type SomeTestSuite struct { *FullFixture }
func (s *SomeTestSuite) TestOne(t *gotest.T) {}
