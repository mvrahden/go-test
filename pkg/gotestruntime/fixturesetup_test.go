package gotestruntime_test

import (
	"context"
	"errors"
	"time"

	"github.com/mvrahden/go-test/pkg/gotest"
	"github.com/mvrahden/go-test/pkg/gotestruntime"
)

// FixtureSetupTestSuite covers the one policy both the in-process fixture DAG
// and the generated shared-fixture subprocess run BeforeAll under.
type FixtureSetupTestSuite struct{}

func (s *FixtureSetupTestSuite) TestPanicIsContained(t *gotest.T) {
	t.When("BeforeAll panics", func(w *gotest.T) {
		w.It("returns an error instead of aborting the process", func(it *gotest.T) {
			err := gotestruntime.RunFixtureSetup(context.Background(), gotestruntime.FixtureSetup{
				Name:      "Panicky",
				BeforeAll: func(ctx context.Context) error { panic("container refused to start") },
			})
			gotest.ErrorContains(it, err, "container refused to start")
		})

		w.It("retries a panic like any other failure", func(it *gotest.T) {
			attempts := 0
			err := gotestruntime.RunFixtureSetup(context.Background(), gotestruntime.FixtureSetup{
				Name:    "Flaky",
				Retries: 2,
				BeforeAll: func(ctx context.Context) error {
					attempts++
					if attempts < 3 {
						panic("not ready yet")
					}
					return nil
				},
			})
			gotest.NoError(it, err)
			gotest.Equal(it, 3, attempts)
		})
	})
}

func (s *FixtureSetupTestSuite) TestBudget(t *gotest.T) {
	t.When("BeforeAll ignores its context and outlives a declared budget", func(w *gotest.T) {
		w.It("fails the attempt", func(it *gotest.T) {
			err := gotestruntime.RunFixtureSetup(context.Background(), gotestruntime.FixtureSetup{
				Name:    "Slow",
				Timeout: 20 * time.Millisecond,
				Budget:  20 * time.Millisecond,
				BeforeAll: func(ctx context.Context) error {
					time.Sleep(60 * time.Millisecond)
					return nil
				},
			})
			gotest.ErrorContains(it, err, "exceeded its configured Timeout")
		})
	})

	t.When("no budget was declared", func(w *gotest.T) {
		w.It("lets the overrun pass", func(it *gotest.T) {
			err := gotestruntime.RunFixtureSetup(context.Background(), gotestruntime.FixtureSetup{
				Name:    "Slow",
				Timeout: 20 * time.Millisecond,
				BeforeAll: func(ctx context.Context) error {
					time.Sleep(60 * time.Millisecond)
					return nil
				},
			})
			gotest.NoError(it, err)
		})
	})
}

func (s *FixtureSetupTestSuite) TestConfigDerivation(t *gotest.T) {
	t.When("the config marker panics", func(w *gotest.T) {
		w.It("returns an error attributed to the fixture", func(it *gotest.T) {
			_, err := gotestruntime.DeriveFixtureConfig("Broken", func() gotest.FixtureConfig {
				panic("env var not set")
			})
			gotest.ErrorContains(it, err, "env var not set")
		})
	})

	t.When("the config marker returns normally", func(w *gotest.T) {
		w.It("hands the config through", func(it *gotest.T) {
			cfg, err := gotestruntime.DeriveFixtureConfig("Configured", func() gotest.FixtureConfig {
				return gotest.FixtureConfig{Timeout: 5 * time.Second}
			})
			gotest.NoError(it, err)
			gotest.Equal(it, 5*time.Second, cfg.Timeout)
		})
	})
}

func (s *FixtureSetupTestSuite) TestParentCancellation(t *gotest.T) {
	t.When("the parent context is cancelled between attempts", func(w *gotest.T) {
		w.It("stops retrying and reports the cancellation", func(it *gotest.T) {
			ctx, cancel := context.WithCancel(context.Background())
			err := gotestruntime.RunFixtureSetup(ctx, gotestruntime.FixtureSetup{
				Name:    "Cancelled",
				Retries: 5,
				BeforeAll: func(attemptCtx context.Context) error {
					cancel()
					return errors.New("first attempt failed")
				},
			})
			gotest.ErrorIs(it, err, context.Canceled)
		})
	})
}
