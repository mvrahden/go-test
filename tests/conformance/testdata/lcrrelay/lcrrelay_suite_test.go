package lcrrelay

import (
	"bufio"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"syscall"
	"time"

	"github.com/mvrahden/go-test/pkg/gotest"
)

// HoldTestSuite holds in its first method until its context ends; the two
// after it never start when the run is stopped meanwhile. The method marks
// every SIGINT or SIGTERM its process gets, and starts a process in its
// group that marks the ones it gets.
type HoldTestSuite struct {
	Hold *HoldFixture
}

func (s *HoldTestSuite) BeforeAll(t *gotest.T) {}
func (s *HoldTestSuite) AfterAll(t *gotest.T)  { mark("suite.AfterAll") }

func (s *HoldTestSuite) TestHolds(t *gotest.T) {
	signals := make(chan os.Signal, 4)
	signal.Notify(signals, syscall.SIGINT, syscall.SIGTERM)
	defer signal.Stop(signals)
	child := exec.Command("sh", "-c", `trap 'echo "child SIGINT" >> "$GOTEST_CANARY_DIR/markers"; exit 0' INT; trap 'echo "child SIGTERM" >> "$GOTEST_CANARY_DIR/markers"; exit 0' TERM; echo ready; i=0; while [ $i -lt 600 ]; do sleep 0.1; i=$((i+1)); done`)
	if startTrapped(child) {
		mark("child started")
	}
	mark("running")
	select {
	case <-t.Context().Done():
		mark("context ended")
	case <-time.After(time.Minute):
	}
	select {
	case sig := <-signals:
		mark("suite " + sig.String())
	case <-time.After(time.Second):
	}
}

func (s *HoldTestSuite) TestSecond(t *gotest.T) {}
func (s *HoldTestSuite) TestThird(t *gotest.T)  {}

// WaitTestSuite's first method waits for a release file in the markers
// directory, a minute at most; the methods after it, and the rows of the
// last, pass once it is there.
type WaitTestSuite struct{}

func (s *WaitTestSuite) TestWaits(t *gotest.T) {
	mark("running")
	release := filepath.Join(os.Getenv("GOTEST_CANARY_DIR"), "release")
	for deadline := time.Now().Add(time.Minute); time.Now().Before(deadline); time.Sleep(20 * time.Millisecond) {
		if _, err := os.Stat(release); err == nil {
			break
		}
	}
	t.T().Log("released")
}

func (s *WaitTestSuite) TestAfter(t *gotest.T) {
	t.It("passes once released", func(it *gotest.T) {})
}

func (s *WaitTestSuite) TestRows(t *gotest.T) {
	for range gotest.Each(t, []struct{ Name string }{{Name: "first"}, {Name: "second"}}) {
	}
}

// PauseTestSuite's first method holds for 2s unless its context ends
// first; the method after it passes.
type PauseTestSuite struct{}

func (s *PauseTestSuite) TestPauses(t *gotest.T) {
	mark("running")
	select {
	case <-t.Context().Done():
		mark("context ended")
	case <-time.After(2 * time.Second):
		mark("held")
	}
}

func (s *PauseTestSuite) TestSecond(t *gotest.T) { mark("second") }

// CrashTestSuite's second method ends its process the way LCR_CRASH names:
// exit for os.Exit(3), panic for a panic on a goroutine nobody recovers,
// kill for a SIGKILL the process sends itself.
type CrashTestSuite struct{}

func (s *CrashTestSuite) TestBefore(t *gotest.T) {
	t.It("passes", func(it *gotest.T) {})
}

func (s *CrashTestSuite) TestCrashes(t *gotest.T) {
	t.It("passes before the end", func(it *gotest.T) {})
	switch os.Getenv("LCR_CRASH") {
	case "exit":
		os.Exit(3)
	case "panic":
		go func() { panic("the suite process crashes") }()
		select {}
	case "method":
		panic("the method panics")
	case "kill":
		_ = syscall.Kill(os.Getpid(), syscall.SIGKILL)
		select {}
	}
}

func (s *CrashTestSuite) TestAfter(t *gotest.T) {}

// startTrapped starts child, a shell that prints a line once its traps are
// set, and returns once it has: a signal sent before then finds no trap and
// ends the shell unmarked.
func startTrapped(child *exec.Cmd) bool {
	out, err := child.StdoutPipe()
	if err != nil || child.Start() != nil {
		return false
	}
	_, err = bufio.NewReader(out).ReadString('\n')
	return err == nil
}
