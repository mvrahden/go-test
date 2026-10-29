package shutdown_test

import (
	"time"

	"github.com/mvrahden/go-test/pkg/gotest"
	"github.com/mvrahden/go-test/tests/gotestcli"
)

// ShutdownInterruptTestSuite proves that a run which ends early still tears its
// shared fixtures down: an interrupt exits 130 and a FailFast trip skips the
// remaining methods, and the fixture's AfterAll runs either way. Not
// Exclusive: each interrupt follows a marker, and no verdict times anything.
//
//nolint:lifecycle-pair // BeforeAll's module lives under t.TempDir(), which the framework removes; the binary is the shared fixture's
type ShutdownInterruptTestSuite struct {
	CLI *gotestcli.BinarySharedFixture
	rig rig
}

func (s *ShutdownInterruptTestSuite) SuiteConfig() gotest.SuiteConfig {
	// BeforeAll compiles, so its time is no verdict.
	return gotest.SuiteConfig{Parallel: true, Timeout: 30 * time.Second}
}

func (s *ShutdownInterruptTestSuite) BeforeAll(t *gotest.T) {
	s.rig = stageRig(t, s.CLI, "./slow/", "./failfast/")
}

// The method ignores its context, so the suite process ends by the signal
// after its grace, and the CLI still tears the shared fixture down.
func (s *ShutdownInterruptTestSuite) TestInterrupt(t *gotest.T) {
	markers := t.TempDir()
	run := s.rig.start(t, markers, "slow")
	interruptWhen(t, run, markers, "test-running")
	code := exitCode(t, run)

	t.It("exits 130", func(it *gotest.T) {
		gotest.Equal(it, 130, code)
	})
	t.It("tears the shared fixture down before exiting", func(it *gotest.T) {
		gotest.True(it, marker(markers, "fixture-up"), "the fixture never started")
		gotest.True(it, marker(markers, "fixture-down"), "AfterAll did not run")
	})
}

func (s *ShutdownInterruptTestSuite) TestFailFastTrip(t *gotest.T) {
	markers := t.TempDir()
	code := exitCode(t, s.rig.start(t, markers, "failfast"))

	t.It("exits 1 and skips the methods after the failure", func(it *gotest.T) {
		gotest.Equal(it, 1, code)
		gotest.False(it, marker(markers, "second-ran"), "the method after the failure ran")
	})
	t.It("tears the shared fixture down", func(it *gotest.T) {
		gotest.True(it, marker(markers, "fixture-down"), "AfterAll did not run")
	})
}

// The spec, summary and bench commands run the batch pipeline, not the
// streaming one; an interrupt must mean 130 on both.
func (s *ShutdownInterruptTestSuite) TestInterruptDuringSpec(t *gotest.T) {
	markers := t.TempDir()
	run := s.rig.startWith(t, markers, nil, "spec", "./slow/")
	interruptWhen(t, run, markers, "test-running")
	code := exitCode(t, run)

	t.It("exits 130", func(it *gotest.T) {
		gotest.Equal(it, 130, code)
	})
	t.It("tears the shared fixture down before exiting", func(it *gotest.T) {
		gotest.True(it, marker(markers, "fixture-down"), "AfterAll did not run")
	})
}
