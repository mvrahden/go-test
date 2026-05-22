package testpkg

import (
	"context"
	"github.com/mvrahden/go-test/pkg/gotest"
)

type PGSharedFixture struct{ ConnStr string }
func (f *PGSharedFixture) BeforeAll(ctx context.Context) error { return nil }
func (f *PGSharedFixture) AfterAll(ctx context.Context) error  { return nil }

type RedisSharedFixture struct{ Addr string }
func (f *RedisSharedFixture) BeforeAll(ctx context.Context) error { return nil }
func (f *RedisSharedFixture) AfterAll(ctx context.Context) error  { return nil }

type FullTestSuite struct {
	pg    *PGSharedFixture
	redis *RedisSharedFixture
}
func (s *FullTestSuite) TestOne(t *gotest.T) {}
