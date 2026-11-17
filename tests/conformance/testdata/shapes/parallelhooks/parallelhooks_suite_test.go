package parallelhooks

import (
	"context"

	"github.com/mvrahden/go-test/pkg/gotest"
)

// TableFixture empties its table before every method.
type TableFixture struct{ rows map[string]bool }

func (f *TableFixture) BeforeAll(ctx context.Context) error {
	f.rows = map[string]bool{}
	return nil
}

func (f *TableFixture) BeforeEach(ctx context.Context) error {
	clear(f.rows)
	return nil
}

// RacingTestSuite is parallel: both methods would run the fixture's hook at once.
type RacingTestSuite struct{ Table *TableFixture }

func (s *RacingTestSuite) SuiteConfig() gotest.SuiteConfig {
	return gotest.SuiteConfig{Parallel: true}
}

func (s *RacingTestSuite) TestA(t *gotest.T) {}

func (s *RacingTestSuite) TestB(t *gotest.T) {}
