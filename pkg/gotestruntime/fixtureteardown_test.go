package gotestruntime_test

import (
	"context"
	"errors"
	"time"

	"github.com/mvrahden/go-test/pkg/gotest"
	"github.com/mvrahden/go-test/pkg/gotestruntime"
)

// FixtureTeardownTestSuite covers the one policy both the in-process fixture
// DAG and the generated shared-fixture subprocess run AfterAll under.
// Sequential: asserts wall-clock budgets.
type FixtureTeardownTestSuite struct{}

func (s *FixtureTeardownTestSuite) TestPanicIsContained(t *gotest.T) {
	t.When("AfterAll panics", func(w *gotest.T) {
		w.It("reports a failure instead of aborting the process", func(it *gotest.T) {
			failed := gotestruntime.RunFixtureTeardown(context.Background(), gotestruntime.FixtureTeardown{
				Name:     "Panicky",
				AfterAll: func(ctx context.Context) error { panic("connection already closed") },
			})
			gotest.True(it, failed)
		})
	})
}

func (s *FixtureTeardownTestSuite) TestOutcome(t *gotest.T) {
	t.When("AfterAll returns an error", func(w *gotest.T) {
		w.It("reports a failure", func(it *gotest.T) {
			failed := gotestruntime.RunFixtureTeardown(context.Background(), gotestruntime.FixtureTeardown{
				Name:     "Broken",
				AfterAll: func(ctx context.Context) error { return errors.New("volume still mounted") },
			})
			gotest.True(it, failed)
		})
	})

	t.When("there is no AfterAll", func(w *gotest.T) {
		w.It("is nothing to release and reports success", func(it *gotest.T) {
			failed := gotestruntime.RunFixtureTeardown(context.Background(), gotestruntime.FixtureTeardown{
				Name: "Stateless",
			})
			gotest.False(it, failed)
		})
	})
}

func (s *FixtureTeardownTestSuite) TestBudget(t *gotest.T) {
	t.When("AfterAll ignores its context and outlives a declared budget", func(w *gotest.T) {
		w.It("fails the teardown", func(it *gotest.T) {
			failed := gotestruntime.RunFixtureTeardown(context.Background(), gotestruntime.FixtureTeardown{
				Name:    "Slow",
				Timeout: 20 * time.Millisecond,
				Budget:  20 * time.Millisecond,
				AfterAll: func(ctx context.Context) error {
					time.Sleep(60 * time.Millisecond)
					return nil
				},
			})
			gotest.True(it, failed)
		})
	})

	t.When("no budget was declared", func(w *gotest.T) {
		w.It("lets the overrun pass", func(it *gotest.T) {
			failed := gotestruntime.RunFixtureTeardown(context.Background(), gotestruntime.FixtureTeardown{
				Name:    "Slow",
				Timeout: 20 * time.Millisecond,
				AfterAll: func(ctx context.Context) error {
					time.Sleep(60 * time.Millisecond)
					return nil
				},
			})
			gotest.False(it, failed)
		})
	})

	t.When("AfterAll finishes within a declared budget", func(w *gotest.T) {
		w.It("reports success", func(it *gotest.T) {
			failed := gotestruntime.RunFixtureTeardown(context.Background(), gotestruntime.FixtureTeardown{
				Name:     "Prompt",
				Timeout:  time.Second,
				Budget:   time.Second,
				AfterAll: func(ctx context.Context) error { return nil },
			})
			gotest.False(it, failed)
		})
	})
}

func (s *FixtureTeardownTestSuite) TestSetupBudget(t *gotest.T) {
	t.When("every attempt and every release between them runs to its Timeout", func(w *gotest.T) {
		w.It("finishes within the budget the setup reports", func(it *gotest.T) {
			attempts := 0
			setup := gotestruntime.FixtureSetup{
				Name:    "Slow",
				Timeout: 200 * time.Millisecond,
				Retries: 1,
				BeforeAll: func(ctx context.Context) error {
					attempts++
					<-ctx.Done()
					if attempts == 1 {
						return ctx.Err()
					}
					return nil
				},
				AfterAll: func(ctx context.Context) error {
					<-ctx.Done()
					return nil
				},
			}
			start := time.Now()
			gotest.NoError(it, gotestruntime.RunFixtureSetup(context.Background(), setup))
			elapsed := time.Since(start)
			gotest.LessOrEqual(it, elapsed, gotestruntime.SetupBudget(setup)+100*time.Millisecond)
		})
	})

	t.When("there is no AfterAll to release between attempts", func(w *gotest.T) {
		w.It("counts the attempts and the delays between them", func(it *gotest.T) {
			gotest.Equal(it, 35*time.Second, gotestruntime.SetupBudget(gotestruntime.FixtureSetup{
				Timeout:    10 * time.Second,
				Retries:    2,
				RetryDelay: 2500 * time.Millisecond,
				BeforeAll:  func(context.Context) error { return nil },
			}))
		})
	})

	t.When("the Timeout is unbounded", func(w *gotest.T) {
		w.It("has no budget", func(it *gotest.T) {
			gotest.Zero(it, gotestruntime.SetupBudget(gotestruntime.FixtureSetup{Timeout: gotest.NoDeadline, Retries: 1}))
		})
	})
}
