//go:build unix

package gotestrunner_test

import (
	"os"
	"os/exec"
	"path/filepath"
	"time"

	"github.com/mvrahden/go-test/internal/gotestrunner"
	"github.com/mvrahden/go-test/internal/proctree"
	"github.com/mvrahden/go-test/internal/protocol"
	"github.com/mvrahden/go-test/internal/testkit"
	"github.com/mvrahden/go-test/pkg/gotest"
)

// AbandonTestSuite covers what a CLI giving up on its trees does to them.
// Sequential: Abandon acts on every tree this process tracks.
type AbandonTestSuite struct {
	tracked []*abandoned
}

// AfterEach kills what a scenario left running and untracks it.
func (s *AbandonTestSuite) AfterEach(t *gotest.T) {
	for _, a := range s.tracked {
		a.end()
	}
	s.tracked = nil
}

// abandoned is one tracked tree whose root is a shell script.
type abandoned struct {
	cmd   *exec.Cmd
	tree  *proctree.Tree
	done  chan struct{}
	stop  func()
	ready string
}

// startTracked starts script as a tracked tree's root; $0 is a marker path
// inside dir and $1 the tree's stop file, which is empty when stopFile is.
func (s *AbandonTestSuite) startTracked(t *gotest.T, dir, script string, stopFile bool) *abandoned {
	stop := ""
	if stopFile {
		stop = filepath.Join(dir, "stop")
	}
	cmd := exec.Command("/bin/sh", "-c", script, filepath.Join(dir, "marker"), stop) //nolint:gosec // G204: fixed script
	tree := proctree.New(cmd)
	gotest.NoError(t, tree.Start())
	a := &abandoned{cmd: cmd, tree: tree, done: make(chan struct{}), ready: filepath.Join(dir, "marker.ready")}
	go func() {
		_ = cmd.Wait()
		tree.Release()
		close(a.done)
	}()
	a.stop = gotestrunner.ExportTrack(tree, cmd.Process.Pid, stop, a.done)
	s.tracked = append(s.tracked, a)
	gotest.Eventually(t, 10*time.Second, 10*time.Millisecond, func(poll *gotest.R) {
		_, err := os.Stat(a.ready)
		gotest.NoError(poll, err)
	})
	return a
}

// end kills the tree unless it has ended, and untracks it.
func (a *abandoned) end() {
	select {
	case <-a.done:
	default:
		_ = a.tree.Kill()
		<-a.done
	}
	a.stop()
}

func (s *AbandonTestSuite) TestATreeWithoutAStopFileIsAskedFirst(t *gotest.T) {
	dir := t.TempDir()
	// Tears down on SIGTERM, as the shared fixture process does.
	s.startTracked(t, dir, `trap 'sleep 0.3; touch "$0.down"; exit 0' TERM; touch "$0.ready"; while :; do sleep 0.05; done`, false)
	started := time.Now()
	gotestrunner.Abandon(5*time.Second, nil)
	took := time.Since(started)
	_, err := os.Stat(filepath.Join(dir, "marker.down"))

	t.When("the CLI gives up on a tree that has no stop file", func(w *gotest.T) {
		w.It("asks it to stop and waits for it", func(it *gotest.T) {
			gotest.NoError(it, err, "the tree was killed before it tore down")
		})
		w.It("returns once it has ended, not at the bound", func(it *gotest.T) {
			gotest.Less(it, took, 3*time.Second)
		})
	})
}

func (s *AbandonTestSuite) TestAFurtherSignalSkipsTheWait(t *gotest.T) {
	dir := t.TempDir()
	// Publishes its pid as a suite process does and ignores the second stop.
	a := s.startTracked(t, dir, `trap '' TERM; echo $$ > "$1.pid"; touch "$0.ready"; while :; do sleep 0.05; done`, true)
	gotest.Eventually(t, 10*time.Second, 10*time.Millisecond, func(poll *gotest.R) {
		_, err := os.Stat(protocol.StopPIDPath(filepath.Join(dir, "stop")))
		gotest.NoError(poll, err)
	})
	hurry := make(chan struct{})
	time.AfterFunc(300*time.Millisecond, func() { close(hurry) })
	started := time.Now()
	gotestrunner.Abandon(10*time.Second, hurry)
	took := time.Since(started)
	pid := a.cmd.Process.Pid

	t.When("a further signal comes while the CLI waits for a suite that ignores the second stop", func(w *gotest.T) {
		w.It("kills it then instead of waiting out the bound", func(it *gotest.T) {
			gotest.Less(it, took, 3*time.Second)
			gotest.True(it, testkit.EndedWithin(pid, 2*time.Second), "the suite process %d outlived Abandon", pid)
		})
	})
}
