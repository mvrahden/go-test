package testpkg

import (
	"context"

	"github.com/mvrahden/go-test/pkg/gotest"
)

type PoolFixture struct {
	Pool string
}

func (f *PoolFixture) BeforeAll(ctx context.Context) error { return nil }
func (f *PoolFixture) AfterAll(ctx context.Context) error  { return nil }

type ParserTestSuite struct {
	*PoolFixture
}

func (s *ParserTestSuite) BenchmarkParse(b *gotest.B) {
	for b.Loop() {
	}
}
