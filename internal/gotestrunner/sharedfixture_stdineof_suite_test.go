package gotestrunner_test

import (
	"os"
	"path/filepath"
	"time"

	"github.com/mvrahden/go-test/internal/gotestrunner"
	"github.com/mvrahden/go-test/pkg/gotest"
	"github.com/mvrahden/go-test/tests/sharedfixture/fixtures"
)

// SharedFixtureStdinEOFTestSuite: a shared fixture process whose command pipe
// ends tears down and exits, as it does where no death signal tells it the
// runner is gone.
// Sequential: the fixture is configured through the environment.
type SharedFixtureStdinEOFTestSuite struct{}

func (s *SharedFixtureStdinEOFTestSuite) TestAClosedCommandPipeTearsTheFixturesDown(t *gotest.T) {
	marker := filepath.Join(t.TempDir(), "released")
	t.Setenv(fixtures.EnvSlowTeardownDelay, (200 * time.Millisecond).String())
	t.Setenv(fixtures.EnvSlowTeardownMarker, marker)
	proc, err := gotestrunner.StartSharedFixtures(t.Context(), t.TempDir(), slowTeardownFixture(), 30*time.Second)
	gotest.NoError(t, err, "starting the shared fixture subprocess")
	defer func() { _ = proc.Teardown() }()
	gotest.NoError(t, proc.WaitAllReady(t.Context(), 30*time.Second))
	gotest.NoError(t, gotestrunner.ExportCloseStdin(proc))

	t.When("the runner's end of the command pipe closes while a fixture is up", func(w *gotest.T) {
		w.It("the fixture's AfterAll runs, then the process exits", func(it *gotest.T) {
			select {
			case <-gotestrunner.ExportExited(proc):
			case <-time.After(20 * time.Second):
				gotest.Fail(it, "the shared fixture process outlived its command pipe")
			}
			_, statErr := os.Stat(marker)
			gotest.NoError(it, statErr, "the shared fixture process exited without its AfterAll")
		})
	})
}
