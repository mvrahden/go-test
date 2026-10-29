//go:build linux

package fixtures_test

import (
	"errors"
	"syscall"
	"time"

	"github.com/mvrahden/go-test/pkg/gotest"
)

// prSetChildSubreaper is prctl's PR_SET_CHILD_SUBREAPER.
const prSetChildSubreaper = 36

// OrphanedFixtureTestSuite: a shared fixture process whose CLI is gone and
// whose hook never returns ends itself once it has had its budgets. The suite
// process adopts the orphan as a subreaper, so it can read its exit status.
type OrphanedFixtureTestSuite struct {
	cli string
}

// SuiteConfig: locally a method took 33s, 32s of it the budgets waited out;
// the rest seven times slower is 39s, twice that 78s, and the Timeout stays
// above the 90s a method waits for the exit so the wait's verdict is shown.
// BeforeAll builds the CLI, 8.5s cold, so twice that seven times slower is 2m.
func (s *OrphanedFixtureTestSuite) SuiteConfig() gotest.SuiteConfig {
	return gotest.SuiteConfig{Parallel: true, Timeout: 90 * time.Second, SetupTimeout: 2 * time.Minute}
}

func (s *OrphanedFixtureTestSuite) BeforeAll(t *gotest.T) {
	_, _, errno := syscall.RawSyscall(syscall.SYS_PRCTL, prSetChildSubreaper, 1, 0)
	if errno != 0 {
		gotest.NoError(t, errno)
	}
	s.cli = buildKillableCLI(t)
}

// AfterAll has nothing to release: the binary lives in a temp dir.
func (s *OrphanedFixtureTestSuite) AfterAll(t *gotest.T) {}

// reaped waits up to limit for the adopted pid to exit and returns its status
// and when it exited.
func reaped(pid int, limit time.Duration) (syscall.WaitStatus, time.Time, error) {
	type result struct {
		ws  syscall.WaitStatus
		at  time.Time
		err error
	}
	ch := make(chan result, 1)
	go func() {
		var ws syscall.WaitStatus
		for {
			_, err := syscall.Wait4(pid, &ws, 0, nil)
			if errors.Is(err, syscall.EINTR) {
				continue
			}
			ch <- result{ws, time.Now(), err}
			return
		}
	}()
	select {
	case r := <-ch:
		return r.ws, r.at, r.err
	case <-time.After(limit):
		return 0, time.Time{}, errors.New("the shared fixture process did not exit")
	}
}

func (s *OrphanedFixtureTestSuite) TestATeardownThatNeverReturns(t *gotest.T) {
	pid, killed := killedAt(t, s.cli, t.TempDir(), "suite holds", "FIXB_TIMEOUT=1s", "FIXB_TEARDOWN=hang")
	ws, exited, err := reaped(pid, 90*time.Second)
	t.When("the CLI is killed and the shared fixture's AfterAll never returns", func(w *gotest.T) {
		w.It("the fixture process exits with status 1 once the 31s teardown budget it reported has passed", func(it *gotest.T) {
			gotest.NoError(it, err)
			gotest.True(it, ws.Exited(), "status: %v", ws)
			gotest.Equal(it, 1, ws.ExitStatus())
			gotest.GreaterOrEqual(it, exited.Sub(killed), 31*time.Second)
			gotest.Less(it, exited.Sub(killed), 50*time.Second)
		})
	})
	_ = syscall.Kill(pid, syscall.SIGKILL)
}

func (s *OrphanedFixtureTestSuite) TestASetupThatIgnoresItsContext(t *gotest.T) {
	pid, killed := killedAt(t, s.cli, t.TempDir(), "setup ignoring", "FIXB_TIMEOUT=1s", "FIXB_SETUP=ignore")
	ws, exited, err := reaped(pid, 90*time.Second)
	t.When("the CLI is killed while a shared fixture's BeforeAll ignores its context", func(w *gotest.T) {
		w.It("the fixture process exits with status 1 once its 1s setup budget and 31s teardown budget have passed", func(it *gotest.T) {
			gotest.NoError(it, err)
			gotest.True(it, ws.Exited(), "status: %v", ws)
			gotest.Equal(it, 1, ws.ExitStatus())
			gotest.GreaterOrEqual(it, exited.Sub(killed), 32*time.Second)
			gotest.Less(it, exited.Sub(killed), 50*time.Second)
		})
	})
	_ = syscall.Kill(pid, syscall.SIGKILL)
}
