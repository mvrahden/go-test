//go:build unix

package gotestrunner_test

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/mvrahden/go-test/internal/gotestrunner"
	"github.com/mvrahden/go-test/pkg/gotest"
)

// SharedFixtureVerbsTestSuite: the runner stops a shared fixture process by a
// line on its stdin, by an interrupt only when the line cannot be written,
// and gives up on it with a force line.
// Sequential: Abandon acts on every tree this process tracks.
type SharedFixtureVerbsTestSuite struct{}

// answering marks to the file $0 names how it was asked to stop, a stop or
// force line or a signal, and exits 0; past the end of its stdin it waits
// for a signal.
const answering = `trap 'echo signal >> "$0"; exit 0' TERM
printf '{"key":"_done","teardownBudget":"10s"}\n' >&3
while read -r line; do
	case $line in
	*'"verb":"stop"'*) echo stop >> "$0"; exit 0;;
	*'"verb":"force"'*) echo force >> "$0"; exit 0;;
	esac
done
while :; do sleep 0.05; done`

// launchAnswering starts answering as a shared fixture process, its setup
// reported, and returns it with the file it marks.
func launchAnswering(t *gotest.T) (*gotestrunner.SharedFixtureProcess, string) {
	marks := filepath.Join(t.TempDir(), "asked")
	proc, err := gotestrunner.ExportLaunchSharedProcess(context.Background(), t.TempDir(), "sh", "-c", answering, marks)
	gotest.NoError(t, err, "starting the fake shared fixture process")
	gotest.NoError(t, proc.WaitAllReady(t.Context(), 10*time.Second))
	return proc, marks
}

// asked is how the process marked it was asked to stop.
func asked(path string) string {
	data, _ := os.ReadFile(path)
	return strings.TrimSpace(string(data))
}

func (s *SharedFixtureVerbsTestSuite) TestTeardownSendsAStopLine(t *gotest.T) {
	proc, marks := launchAnswering(t)
	err := proc.Teardown()
	t.When("the run tears its shared fixture process down", func(w *gotest.T) {
		w.It("asks by a stop line on its stdin, not by a signal", func(it *gotest.T) {
			gotest.NoError(it, err)
			gotest.Equal(it, "stop", asked(marks))
		})
	})
}

func (s *SharedFixtureVerbsTestSuite) TestAStopLineThatCannotBeWritten(t *gotest.T) {
	proc, marks := launchAnswering(t)
	gotest.NoError(t, gotestrunner.ExportCloseStdin(proc))
	err := proc.Teardown()
	t.When("the stop line cannot be written to the shared fixture process", func(w *gotest.T) {
		w.It("interrupts it instead", func(it *gotest.T) {
			gotest.NoError(it, err)
			gotest.Equal(it, "signal", asked(marks))
		})
	})
}

func (s *SharedFixtureVerbsTestSuite) TestAbandonSendsAForceLine(t *gotest.T) {
	proc, marks := launchAnswering(t)
	started := time.Now()
	gotestrunner.Abandon(5*time.Second, nil)
	took := time.Since(started)
	err := proc.Teardown()
	t.When("the CLI gives up on its shared fixture process", func(w *gotest.T) {
		w.It("asks by a force line and waits for it", func(it *gotest.T) {
			gotest.NoError(it, err)
			gotest.Equal(it, "force", asked(marks))
			gotest.Less(it, took, 3*time.Second)
		})
	})
}
