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
// Sequential: asserts wall-clock budgets.
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

func (s *FixtureSetupTestSuite) TestRetriesReleaseBetweenAttempts(t *gotest.T) {
	t.When("BeforeAll fails and is retried", func(w *gotest.T) {
		w.It("runs AfterAll before each retry, releasing what the failed attempt reached, and not after the success", func(it *gotest.T) {
			var events []string
			attempts := 0
			err := gotestruntime.RunFixtureSetup(context.Background(), gotestruntime.FixtureSetup{
				Name:    "Flaky",
				Retries: 2,
				BeforeAll: func(ctx context.Context) error {
					attempts++
					events = append(events, "before")
					if attempts < 3 {
						return errors.New("not ready yet")
					}
					return nil
				},
				AfterAll: func(ctx context.Context) error {
					events = append(events, "after")
					return nil
				},
			})
			gotest.NoError(it, err)
			gotest.Equal(it, []string{"before", "after", "before", "after", "before"}, events)
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

	t.When("a successful attempt overran its budget", func(w *gotest.T) {
		w.It("is terminal and never retried", func(it *gotest.T) {
			attempts := 0
			err := gotestruntime.RunFixtureSetup(context.Background(), gotestruntime.FixtureSetup{
				Name:    "Slow",
				Timeout: 20 * time.Millisecond,
				Budget:  20 * time.Millisecond,
				Retries: 2,
				BeforeAll: func(ctx context.Context) error {
					attempts++
					time.Sleep(60 * time.Millisecond)
					return nil
				},
			})
			gotest.ErrorIs(it, err, gotestruntime.ErrSetupOverran)
			gotest.Equal(it, 1, attempts,
				"the work completed and the overrun is deterministic; a retry would build a second set of resources")
		})
	})

	t.When("the parent context's deadline fires mid-attempt", func(w *gotest.T) {
		w.It("does not blame the fixture's own budget", func(it *gotest.T) {
			ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
			err := gotestruntime.RunFixtureSetup(ctx, gotestruntime.FixtureSetup{
				Name:    "Prompt",
				Timeout: 500 * time.Millisecond,
				Budget:  500 * time.Millisecond,
				BeforeAll: func(ctx context.Context) error {
					time.Sleep(60 * time.Millisecond)
					return nil
				},
			})
			cancel()
			gotest.NoError(it, err,
				"the caller's deadline is not this fixture's budget; a verdict here names a number the author never blew")
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

func (s *FixtureSetupTestSuite) TestCancellationDuringTheRetryDelay(t *gotest.T) {
	t.When("the parent context is cancelled while the retry delay runs", func(w *gotest.T) {
		w.It("reports the cancellation without running AfterAll, which the caller's teardown runs once", func(it *gotest.T) {
			ctx, cancel := context.WithCancel(context.Background())
			afterAlls := 0
			err := gotestruntime.RunFixtureSetup(ctx, gotestruntime.FixtureSetup{
				Name:       "Delayed",
				Retries:    1,
				RetryDelay: 2 * time.Second,
				BeforeAll: func(context.Context) error {
					time.AfterFunc(50*time.Millisecond, cancel)
					return errors.New("not yet")
				},
				AfterAll: func(context.Context) error {
					afterAlls++
					return nil
				},
			})
			gotest.ErrorIs(it, err, context.Canceled)
			gotest.Zero(it, afterAlls)
		})
	})
}

func (s *FixtureSetupTestSuite) TestReleaseFailureBetweenAttempts(t *gotest.T) {
	t.When("the release between attempts fails and a later attempt succeeds", func(w *gotest.T) {
		w.It("fails the setup with the release's error", func(it *gotest.T) {
			attempts := 0
			err := gotestruntime.RunFixtureSetup(context.Background(), gotestruntime.FixtureSetup{
				Name:    "Leaky",
				Retries: 1,
				BeforeAll: func(context.Context) error {
					attempts++
					if attempts == 1 {
						return errors.New("not ready yet")
					}
					return nil
				},
				AfterAll: func(context.Context) error { return errors.New("container leaked") },
			})
			gotest.ErrorIs(it, err, gotestruntime.ErrReleaseFailed)
			gotest.ErrorContains(it, err, "container leaked")
			gotest.Equal(it, 2, attempts)
		})
	})

	t.When("the release between attempts panics and a later attempt succeeds", func(w *gotest.T) {
		w.It("fails the setup", func(it *gotest.T) {
			attempts := 0
			err := gotestruntime.RunFixtureSetup(context.Background(), gotestruntime.FixtureSetup{
				Name:    "Leaky",
				Retries: 1,
				BeforeAll: func(context.Context) error {
					attempts++
					if attempts == 1 {
						return errors.New("not ready yet")
					}
					return nil
				},
				AfterAll: func(context.Context) error { panic("nil pool") },
			})
			gotest.ErrorIs(it, err, gotestruntime.ErrReleaseFailed)
		})
	})

	t.When("the release between attempts fails and every attempt fails", func(w *gotest.T) {
		w.It("reports both the last attempt's error and the release's", func(it *gotest.T) {
			err := gotestruntime.RunFixtureSetup(context.Background(), gotestruntime.FixtureSetup{
				Name:      "Leaky",
				Retries:   1,
				BeforeAll: func(context.Context) error { return errors.New("not ready yet") },
				AfterAll:  func(context.Context) error { return errors.New("container leaked") },
			})
			gotest.ErrorContains(it, err, "not ready yet")
			gotest.ErrorContains(it, err, "container leaked")
		})
	})
}

func (s *FixtureSetupTestSuite) TestHeld(t *gotest.T) {
	t.When("BeforeAll runs, failing and retried", func(w *gotest.T) {
		w.It("reports the fixture held before each attempt and not held after each release between them", func(it *gotest.T) {
			var events []string
			_ = gotestruntime.RunFixtureSetup(context.Background(), gotestruntime.FixtureSetup{
				Name:    "Flaky",
				Retries: 1,
				Held: func(held bool) {
					if held {
						events = append(events, "held")
					} else {
						events = append(events, "released")
					}
				},
				BeforeAll: func(context.Context) error {
					events = append(events, "before")
					return errors.New("not ready yet")
				},
				AfterAll: func(context.Context) error {
					events = append(events, "after")
					return nil
				},
			})
			gotest.Equal(it, []string{"held", "before", "after", "released", "held", "before"}, events,
				"the last attempt stays held: the caller's teardown releases it")
		})
	})

	t.When("the setup is cancelled before BeforeAll runs", func(w *gotest.T) {
		w.It("never reports the fixture held", func(it *gotest.T) {
			ctx, cancel := context.WithCancel(context.Background())
			cancel()
			held := false
			err := gotestruntime.RunFixtureSetup(ctx, gotestruntime.FixtureSetup{
				Name:      "Cancelled",
				Held:      func(h bool) { held = held || h },
				BeforeAll: func(context.Context) error { return nil },
			})
			gotest.ErrorIs(it, err, context.Canceled)
			gotest.False(it, held)
		})
	})
}

func (s *FixtureSetupTestSuite) TestCancellationDuringTheRelease(t *gotest.T) {
	t.When("the parent context is cancelled while the release between attempts runs", func(w *gotest.T) {
		w.It("hands AfterAll a context the cancel does not reach, enters no further attempt, and leaves nothing held", func(it *gotest.T) {
			ctx, cancel := context.WithCancel(context.Background())
			attempts := 0
			var releaseCtxErr error
			var held []bool
			err := gotestruntime.RunFixtureSetup(ctx, gotestruntime.FixtureSetup{
				Name:    "Interrupted",
				Retries: 2,
				Held:    func(h bool) { held = append(held, h) },
				BeforeAll: func(context.Context) error {
					attempts++
					return errors.New("not ready yet")
				},
				AfterAll: func(releaseCtx context.Context) error {
					cancel()
					releaseCtxErr = releaseCtx.Err()
					return nil
				},
			})
			gotest.ErrorIs(it, err, context.Canceled)
			gotest.NoError(it, releaseCtxErr)
			gotest.Equal(it, 1, attempts)
			gotest.Equal(it, []bool{true, false}, held)
		})

		w.It("reports the release's failure beside the cancellation", func(it *gotest.T) {
			ctx, cancel := context.WithCancel(context.Background())
			err := gotestruntime.RunFixtureSetup(ctx, gotestruntime.FixtureSetup{
				Name:      "Interrupted",
				Retries:   2,
				BeforeAll: func(context.Context) error { return errors.New("not ready yet") },
				AfterAll: func(context.Context) error {
					cancel()
					return errors.New("container leaked")
				},
			})
			gotest.ErrorIs(it, err, context.Canceled)
			gotest.ErrorIs(it, err, gotestruntime.ErrReleaseFailed)
			gotest.ErrorContains(it, err, "container leaked")
		})
	})

	t.When("the parent context is cancelled during the attempt after a failed release", func(w *gotest.T) {
		w.It("reports the release's failure beside the cancellation", func(it *gotest.T) {
			ctx, cancel := context.WithCancel(context.Background())
			attempts := 0
			err := gotestruntime.RunFixtureSetup(ctx, gotestruntime.FixtureSetup{
				Name:    "Interrupted",
				Retries: 2,
				BeforeAll: func(context.Context) error {
					attempts++
					if attempts == 2 {
						cancel()
					}
					return errors.New("not ready yet")
				},
				AfterAll: func(context.Context) error { return errors.New("container leaked") },
			})
			gotest.ErrorIs(it, err, context.Canceled)
			gotest.ErrorIs(it, err, gotestruntime.ErrReleaseFailed)
		})
	})
}

func (s *FixtureSetupTestSuite) TestReleaseBudget(t *gotest.T) {
	t.When("a fixture declares a Timeout", func(w *gotest.T) {
		w.It("bounds the release between attempts' context by it", func(it *gotest.T) {
			var hasDeadline bool
			_ = gotestruntime.RunFixtureSetup(context.Background(), gotestruntime.FixtureSetup{
				Name:      "Bounded",
				Timeout:   time.Minute,
				Budget:    time.Minute,
				Retries:   1,
				BeforeAll: func(context.Context) error { return errors.New("not ready yet") },
				AfterAll: func(ctx context.Context) error {
					_, hasDeadline = ctx.Deadline()
					return nil
				},
			})
			gotest.True(it, hasDeadline)
		})
	})

	t.When("the release between attempts ignores its context and outlives the declared Timeout", func(w *gotest.T) {
		w.It("fails the setup though the retry succeeds", func(it *gotest.T) {
			attempts := 0
			err := gotestruntime.RunFixtureSetup(context.Background(), gotestruntime.FixtureSetup{
				Name:    "SlowRelease",
				Timeout: 20 * time.Millisecond,
				Budget:  20 * time.Millisecond,
				Retries: 1,
				BeforeAll: func(context.Context) error {
					attempts++
					if attempts == 1 {
						return errors.New("not ready yet")
					}
					return nil
				},
				AfterAll: func(context.Context) error {
					time.Sleep(60 * time.Millisecond)
					return nil
				},
			})
			gotest.ErrorIs(it, err, gotestruntime.ErrReleaseFailed)
			gotest.ErrorContains(it, err, "exceeded its configured Timeout")
			gotest.Equal(it, 2, attempts)
		})
	})

	t.When("a retry overruns its budget after a failed release", func(w *gotest.T) {
		w.It("reports the release's failure beside the overrun", func(it *gotest.T) {
			attempts := 0
			err := gotestruntime.RunFixtureSetup(context.Background(), gotestruntime.FixtureSetup{
				Name:    "Slow",
				Timeout: 20 * time.Millisecond,
				Budget:  20 * time.Millisecond,
				Retries: 1,
				BeforeAll: func(context.Context) error {
					attempts++
					if attempts == 1 {
						return errors.New("not ready yet")
					}
					time.Sleep(60 * time.Millisecond)
					return nil
				},
				AfterAll: func(context.Context) error { return errors.New("container leaked") },
			})
			gotest.ErrorIs(it, err, gotestruntime.ErrSetupOverran)
			gotest.ErrorIs(it, err, gotestruntime.ErrReleaseFailed)
		})
	})
}
