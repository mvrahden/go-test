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

type ParserFuzzTestSuite struct {
	*PoolFixture
}

func (s *ParserFuzzTestSuite) FuzzParse(f *gotest.F) {
	gotest.Fuzz(f, func(t *gotest.T, in string) {})
}
