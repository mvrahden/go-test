// Package slowsetup has a shared fixture that comes up slowly, within the
// budget it declares.
package slowsetup

import (
	"context"
	"time"

	"github.com/mvrahden/go-test/pkg/gotest"
)

// ContainerSharedFixture declares ContainerFixtureConfig and takes 35s to
// come up, past the runner's 30s wait for a first reported budget.
type ContainerSharedFixture struct {
	State string
}

func (f *ContainerSharedFixture) SharedFixtureConfig() gotest.FixtureConfig {
	return gotest.ContainerFixtureConfig()
}

func (f *ContainerSharedFixture) BeforeAll(ctx context.Context) error {
	select {
	case <-time.After(35 * time.Second):
	case <-ctx.Done():
		return ctx.Err()
	}
	f.State = "up"
	return nil
}

func (f *ContainerSharedFixture) AfterAll(ctx context.Context) error { return nil }
