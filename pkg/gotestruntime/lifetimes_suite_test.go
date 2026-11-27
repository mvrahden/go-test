// Ring 0: raw checks only (see ring0_suite_test.go).
package gotestruntime_test //nolint:fail-guard // ring 0: raw checks only, never a gotest assertion

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"testing"
	"time"

	"github.com/mvrahden/go-test/pkg/gotest"
	"github.com/mvrahden/go-test/pkg/gotestruntime"
)

// LifetimesTestSuite: how long what a hook takes from its T lasts.
// Sequential: BeforeAll sets an environment variable, which testing refuses
// below a parallel test.
type LifetimesTestSuite struct{}

func (s *LifetimesTestSuite) TestBeforeAllResourcesLastThroughAfterAll(t *gotest.T) {
	const key = "GOTEST_LIFETIMES_PROBE"
	var dir, env string
	var statErr error
	t.T().Run("inner", func(tt *testing.T) { //nolint:t-escape // OpenSuite takes the raw testing.TB
		gotestruntime.OpenSuite(tt, gotestruntime.Suite{
			BeforeAll: func(bt *gotest.T) { dir = bt.TempDir(); bt.Setenv(key, "set") },
			AfterAll:  func(*gotest.T) { _, statErr = os.Stat(dir); env = os.Getenv(key) },
		})
	})
	if statErr != nil {
		t.Errorf("AfterAll found BeforeAll's temp dir gone: %v", statErr)
	}
	if env != "set" {
		t.Errorf("AfterAll found BeforeAll's environment variable undone: %q", env)
	}
	if _, err := os.Stat(dir); !os.IsNotExist(err) {
		t.Errorf("BeforeAll's temp dir outlived the suite: %v", err)
	}
	if got := os.Getenv(key); got != "" {
		t.Errorf("BeforeAll's environment variable outlived the suite: %q", got)
	}
}

func (s *LifetimesTestSuite) TestBeforeAllContextOutlivesItsSetupTimeout(t *gotest.T) {
	var ctx context.Context
	var during, after bool
	var inMethods, inAfterAll error
	t.T().Run("inner", func(tt *testing.T) { //nolint:t-escape // OpenSuite takes the raw testing.TB
		gotestruntime.OpenSuite(tt, gotestruntime.Suite{
			Config:    func() gotest.SuiteConfig { return gotest.SuiteConfig{SetupTimeout: 50 * time.Millisecond} },
			BeforeAll: func(bt *gotest.T) { ctx = bt.Context(); _, during = ctx.Deadline() },
			AfterAll:  func(*gotest.T) { inAfterAll = ctx.Err() },
		})
		time.Sleep(150 * time.Millisecond) // the methods' time: three SetupTimeouts
		inMethods = ctx.Err()
		_, after = ctx.Deadline()
	})
	if !during {
		t.Errorf("BeforeAll's context carried no deadline while BeforeAll ran")
	}
	if inMethods != nil {
		t.Errorf("BeforeAll's context ended while the suite still ran: %v", inMethods)
	}
	if after {
		t.Errorf("BeforeAll's context still reports a deadline after BeforeAll returned")
	}
	if inAfterAll == nil {
		t.Errorf("BeforeAll's context was still live in AfterAll")
	}
}

func (s *LifetimesTestSuite) TestBeforeAllContextEndsAtTheSetupTimeoutWhileItRuns(t *gotest.T) {
	var err error
	t.T().Run("inner", func(tt *testing.T) { //nolint:t-escape // RunSetup takes the raw testing.TB
		// No declared budget: the context ends, nothing is failed.
		gotestruntime.RunSetup(tt, 50*time.Millisecond, 0, func(bt *gotest.T) {
			select {
			case <-bt.Context().Done():
				err = bt.Context().Err()
			case <-time.After(2 * time.Second):
			}
		})
	})
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Errorf("BeforeAll's context did not end at its SetupTimeout: %v", err)
	}
}

func (s *LifetimesTestSuite) TestAFixtureAfterEachContextEndsAtTheTimeout(t *gotest.T) {
	var bounded bool
	var left time.Duration
	t.T().Run("inner", func(tt *testing.T) { //nolint:t-escape // RunMethods takes the raw testing.T
		cfg := gotest.SuiteConfig{Timeout: 5 * time.Second}
		gotestruntime.RunMethods(tt, cfg, cfg, gotestruntime.Methods{
			Fixtures: []gotestruntime.EachHooks{{Name: "Probe", After: func(ctx context.Context) error {
				deadline, ok := ctx.Deadline()
				bounded, left = ok, time.Until(deadline)
				return nil
			}}},
			Cases: []gotestruntime.Method{{Name: "TestOne", Run: func(mt *gotest.T, run func(func())) {
				mt.AssertsBy(underTest)
				run(func() {})
			}}},
		})
	})
	if !bounded || left <= 0 || left > 5*time.Second {
		t.Errorf("a fixture AfterEach got a context without the method's deadline (deadline %v, %v left)", bounded, left)
	}
}

func (s *LifetimesTestSuite) TestAFixtureAfterEachContextOutlivesTheDefaultDeadline(t *gotest.T) {
	var bounded bool
	var err error
	passed := t.T().Run("inner", func(tt *testing.T) { //nolint:t-escape // RunMethods takes the raw testing.T
		// The effective Timeout, with none declared: the default deadline.
		cfg := gotest.SuiteConfig{Timeout: 50 * time.Millisecond}
		gotestruntime.RunMethods(tt, cfg, gotest.SuiteConfig{}, gotestruntime.Methods{
			Fixtures: []gotestruntime.EachHooks{{Name: "Probe", After: func(ctx context.Context) error {
				_, bounded = ctx.Deadline()
				err = ctx.Err()
				return err
			}}},
			Cases: []gotestruntime.Method{{Name: "TestSlow", Run: func(mt *gotest.T, run func(func())) {
				mt.AssertsBy(underTest)
				run(func() { time.Sleep(150 * time.Millisecond) })
			}}},
		})
	})
	if bounded || err != nil {
		t.Errorf("a fixture AfterEach got a context the default deadline ends (deadline %v, err %v)", bounded, err)
	}
	if !passed {
		t.Errorf("a method past the default deadline failed at its fixture's AfterEach")
	}
}

// overrunChildEnv makes the test below the child that overruns the methods.
const overrunChildEnv = "GOTEST_LIFETIMES_OVERRUN_CHILD"

// The fixture AfterEach after a BeforeEach that waited out the Timeout must
// find its own context ended too. A miss is rare, so the child overruns a
// thousand methods; every one of them fails, so it runs in a process of its own.
// A method whose Timeout passed before its fixture's BeforeEach started runs
// neither hook, and is not counted.
func (s *LifetimesTestSuite) TestAFixtureAfterEachContextHasEndedOnceItsBeforeEachSawTheTimeout(t *gotest.T) {
	t.AssertsBy(rawCheck)
	if os.Getenv(overrunChildEnv) != "" {
		ran, live := 0, 0
		for range 1000 {
			var err error
			after := false
			t.T().Run("overrun", func(tt *testing.T) { //nolint:t-escape // RunMethods takes the raw testing.T
				cfg := gotest.SuiteConfig{Timeout: 2 * time.Millisecond}
				gotestruntime.RunMethods(tt, cfg, cfg, gotestruntime.Methods{
					Fixtures: []gotestruntime.EachHooks{{
						Name:   "Probe",
						Before: func(ctx context.Context) error { <-ctx.Done(); return nil },
						After:  func(ctx context.Context) error { after, err = true, ctx.Err(); return nil },
					}},
					Cases: []gotestruntime.Method{{Name: "TestOne", Run: func(mt *gotest.T, run func(func())) {
						mt.AssertsBy(underTest)
						run(func() {})
					}}},
				})
			})
			if after {
				ran++
				if err == nil {
					live++
				}
			}
		}
		fmt.Printf("ran=%d live=%d\n", ran, live)
		return
	}
	cmd := exec.Command(os.Args[0], "-test.run=^TestLifetimesTestSuite$/^TestAFixtureAfterEachContextHasEndedOnceItsBeforeEachSawTheTimeout$", "-test.count=1") //nolint:gosec // G204: this test binary
	cmd.Env = append(os.Environ(), overrunChildEnv+"=1")
	out, _ := cmd.CombinedOutput() // the child fails: every method overran
	line := grepLine(string(out), "live=")
	if !strings.Contains(line, " live=0\n") || strings.HasPrefix(line, "ran=0 ") {
		t.Errorf("a fixture AfterEach saw a live context after its BeforeEach saw the Timeout, or none ran:\n%s", line)
	}
}

// grepLine returns the first line of out holding substr, or all of out.
func grepLine(out, substr string) string {
	for line := range strings.Lines(out) {
		if strings.Contains(line, substr) {
			return line
		}
	}
	return out
}
