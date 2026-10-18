package gotestruntime_test

import (
	"bytes"
	"time"

	"github.com/mvrahden/go-test/pkg/gotest"
	"github.com/mvrahden/go-test/pkg/gotestruntime"
)

// SharedLifetimeTestSuite: a shared fixture process whose runner is gone ends
// once it has had its setup and teardown budgets, and says what it may leave
// behind.
type SharedLifetimeTestSuite struct{}

// orphaned is what ExitWhenOrphaned did once the runner was gone.
type orphaned struct {
	slept  time.Duration
	status int
	stderr string
}

func runOrphaned(l *gotestruntime.SharedLifetime, stderr *bytes.Buffer, out *orphaned) {
	gone := make(chan struct{})
	close(gone)
	l.ExitWhenOrphaned(gone)
	out.stderr = stderr.String()
}

func newLifetime(out *orphaned, stderr *bytes.Buffer, names ...string) *gotestruntime.SharedLifetime {
	out.status = -1
	return gotestruntime.ExportSharedLifetime(stderr,
		func(code int) { out.status = code },
		func(d time.Duration) { out.slept = d },
		time.Second, names...)
}

func (s *SharedLifetimeTestSuite) TestASetupRunningGetsItsBudgetBeforeTeardown(t *gotest.T) {
	var out orphaned
	var stderr bytes.Buffer
	l := newLifetime(&out, &stderr, "DBFixture", "CacheFixture")
	l.SetupBegins(0, gotestruntime.FixtureSetup{Timeout: 3 * time.Second, Retries: 1, RetryDelay: time.Second})
	runOrphaned(l, &stderr, &out)

	t.When("the runner is gone while a setup with a declared Timeout runs", func(w *gotest.T) {
		w.It("waits the setup budget plus the teardown budget, then exits with status 1", func(it *gotest.T) {
			// Setup: 3s twice plus a 1s delay. Teardown: 3s, the other
			// fixture's default 2m, and 1s headroom.
			gotest.Equal(it, 7*time.Second+2*time.Minute+4*time.Second, out.slept)
			gotest.Equal(it, 1, out.status)
		})
		w.It("names the fixture whose setup began", func(it *gotest.T) {
			gotest.Equal(it, "gotest: the runner is gone and shared fixture setup and teardown did not finish within 2m11s"+
				" (setup budget 7s plus teardown budget 2m4s); what DBFixture holds may be left behind\n", out.stderr)
		})
	})
}

func (s *SharedLifetimeTestSuite) TestASetupWithoutADeadlineGetsAFixedBound(t *gotest.T) {
	var out orphaned
	var stderr bytes.Buffer
	l := newLifetime(&out, &stderr, "DBFixture")
	l.SetupBegins(0, gotestruntime.FixtureSetup{Timeout: gotest.NoDeadline})
	runOrphaned(l, &stderr, &out)

	t.When("the runner is gone while a setup that declared no deadline runs", func(w *gotest.T) {
		w.It("gives the setup a fixed bound and says so", func(it *gotest.T) {
			gotest.Equal(it, 2*time.Minute+2*time.Minute+time.Second, out.slept)
			gotest.Equal(it, 1, out.status)
			gotest.Equal(it, "gotest: the runner is gone and shared fixture setup and teardown did not finish within 4m1s"+
				" (a setup that declared no deadline was given a fixed 2m0s, plus teardown budget 2m1s); what DBFixture holds may be left behind\n", out.stderr)
		})
	})
}

func (s *SharedLifetimeTestSuite) TestTeardownGetsTheBudgetTheRunnerWasTold(t *gotest.T) {
	var out orphaned
	var stderr bytes.Buffer
	l := newLifetime(&out, &stderr, "DBFixture", "CacheFixture")
	for i := range 2 {
		l.SetupBegins(i, gotestruntime.FixtureSetup{Timeout: time.Second})
		l.SetupEnds(i)
	}
	l.Reported(45 * time.Second)
	l.Released(1)
	runOrphaned(l, &stderr, &out)

	t.When("the runner is gone after setup, with one fixture released", func(w *gotest.T) {
		w.It("waits the reported teardown budget and names only what is still held", func(it *gotest.T) {
			gotest.Equal(it, 45*time.Second, out.slept)
			gotest.Equal(it, 1, out.status)
			gotest.Equal(it, "gotest: the runner is gone and shared fixture teardown did not finish within 45s; what DBFixture holds may be left behind\n", out.stderr)
		})
	})
}
