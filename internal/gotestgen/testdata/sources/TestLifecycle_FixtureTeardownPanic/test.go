package testpkg

import (
	"context"
	"fmt"
	"time"

	"github.com/mvrahden/go-test/pkg/gotest"
)

// SlowReleaseFixture models teardown that takes real time, so a sibling's panic
// would reliably kill it if teardown were unguarded.
type SlowReleaseFixture struct{}

func (f *SlowReleaseFixture) BeforeAll(_ context.Context) error { return nil }
func (f *SlowReleaseFixture) AfterAll(_ context.Context) error {
	time.Sleep(200 * time.Millisecond)
	fmt.Println("MARK:slow fixture released")
	return nil
}

type PanickyTeardownFixture struct{}

func (f *PanickyTeardownFixture) BeforeAll(_ context.Context) error { return nil }
func (f *PanickyTeardownFixture) AfterAll(_ context.Context) error {
	panic("boom in fixture AfterAll")
}

// TeardownPanicTestSuite binds both fixtures so their teardowns race.
type TeardownPanicTestSuite struct {
	*SlowReleaseFixture
	*PanickyTeardownFixture
}

func (s *TeardownPanicTestSuite) TestOne(t *gotest.T) {
	fmt.Println("MARK:test ran")
}
