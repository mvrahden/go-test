package testpkg

import (
	"context"

	"github.com/mvrahden/go-test/pkg/gotest"
)

type HookedFixture struct{}

func (f *HookedFixture) BeforeAll(ctx context.Context) error  { return nil }
func (f *HookedFixture) AfterAll(ctx context.Context) error   { return nil }
func (f *HookedFixture) BeforeEach(ctx context.Context) error { return nil }

type WorkerTestSuite struct {
	*HookedFixture
}

func (s *WorkerTestSuite) BenchmarkWork(b *gotest.B) {
	for b.Loop() {
	}
}
