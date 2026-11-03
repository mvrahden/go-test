package gotestrunner_test

import (
	"bytes"
	"sync"
	"time"

	"github.com/mvrahden/go-test/internal/gotestrunner"
	"github.com/mvrahden/go-test/pkg/gotest"
)

// ExitWritesTestSuite: a CLI about to exit lets the event on its way reach
// stdout and the events file both, and writes no other.
// Sequential: the exit it observes is the process's.
type ExitWritesTestSuite struct{}

// heldWriter is an events file whose writes wait for release.
type heldWriter struct {
	entered  chan struct{}
	release  chan struct{}
	once     sync.Once
	released sync.Once
	buf      bytes.Buffer
}

func newHeldWriter() *heldWriter {
	return &heldWriter{entered: make(chan struct{}), release: make(chan struct{})}
}

// letGo releases every write, once; deferred, so a failed step leaves no
// writer waiting.
func (h *heldWriter) letGo() { h.released.Do(func() { close(h.release) }) }

func (h *heldWriter) Write(p []byte) (int, error) {
	h.once.Do(func() { close(h.entered) })
	<-h.release
	return h.buf.Write(p)
}

const interruptedEvents = `{"Action":"start","Package":"interrupted"}
{"Action":"output","Package":"interrupted","Output":"FAIL: run interrupted while running: example.com/p TestS/TestHolds\n"}
{"Action":"fail","Package":"interrupted"}
`

func (s *ExitWritesTestSuite) TestEndWrites(t *gotest.T) {
	t.When("an event is on its way to the events file", func(w *gotest.T) {
		w.It("returns once it is there, the file reading as stdout does", func(it *gotest.T) {
			var stdout bytes.Buffer
			file := newHeldWriter()
			defer file.letGo()
			gate := &gotestrunner.WriteGate{}
			c := gotestrunner.NewOutputCollector(gotestrunner.RunStreamJSON, false,
				gotestrunner.WithWriters(&stdout, &bytes.Buffer{}), gotestrunner.WithEvents(file), gotestrunner.WithWriteGate(gate))
			wait := gotest.Go(it, func() { gotestrunner.WriteEvents(c, []byte(interruptedEvents)) })
			select {
			case <-file.entered:
			case <-time.After(5 * time.Second):
				gotest.Fail(it, "the event never reached the events file")
			}
			returned := make(chan bool, 1)
			gotest.Go(it, func() { returned <- gotestrunner.EndWritesOf(gate, time.Minute, nil) })
			select {
			case <-returned:
				gotest.Fail(it, "EndWrites returned while the event was on its way to the file")
			case <-time.After(100 * time.Millisecond):
			}
			file.letGo()
			select {
			case ended := <-returned:
				gotest.True(it, ended)
			case <-time.After(5 * time.Second):
				gotest.Fail(it, "EndWrites did not return once the event was in the file")
			}
			wait()
			gotest.Equal(it, interruptedEvents, stdout.String())
			gotest.Equal(it, stdout.String(), file.buf.String())
		})
		w.It("gives up at its bound when the write never ends", func(it *gotest.T) {
			var stdout bytes.Buffer
			file := newHeldWriter()
			defer file.letGo()
			gate := &gotestrunner.WriteGate{}
			c := gotestrunner.NewOutputCollector(gotestrunner.RunStreamJSON, false,
				gotestrunner.WithWriters(&stdout, &bytes.Buffer{}), gotestrunner.WithEvents(file), gotestrunner.WithWriteGate(gate))
			wait := gotest.Go(it, func() { gotestrunner.WriteEvents(c, []byte(interruptedEvents)) })
			<-file.entered
			began := time.Now()
			gotest.False(it, gotestrunner.EndWritesOf(gate, 50*time.Millisecond, nil))
			gotest.Less(it, time.Since(began), 5*time.Second)
			file.letGo()
			wait()
		})
		w.It("gives up on a further signal", func(it *gotest.T) {
			var stdout bytes.Buffer
			file := newHeldWriter()
			defer file.letGo()
			gate := &gotestrunner.WriteGate{}
			c := gotestrunner.NewOutputCollector(gotestrunner.RunStreamJSON, false,
				gotestrunner.WithWriters(&stdout, &bytes.Buffer{}), gotestrunner.WithEvents(file), gotestrunner.WithWriteGate(gate))
			wait := gotest.Go(it, func() { gotestrunner.WriteEvents(c, []byte(interruptedEvents)) })
			<-file.entered
			hurry := make(chan struct{})
			close(hurry)
			began := time.Now()
			gotest.False(it, gotestrunner.EndWritesOf(gate, time.Minute, hurry))
			gotest.Less(it, time.Since(began), 5*time.Second)
			file.letGo()
			wait()
		})
	})
	t.When("the process is exited while an event is on its way", func(w *gotest.T) {
		w.It("ends once the event reached both, not between them", func(it *gotest.T) {
			var stdout bytes.Buffer
			file := newHeldWriter()
			defer file.letGo()
			c := gotestrunner.NewOutputCollector(gotestrunner.RunStreamJSON, false,
				gotestrunner.WithWriters(&stdout, &bytes.Buffer{}), gotestrunner.WithEvents(file))
			wait := gotest.Go(it, func() { gotestrunner.WriteEvents(c, []byte(interruptedEvents)) })
			<-file.entered
			exited := make(chan string, 1)
			restore := gotestrunner.ExportObserveExit(func(int) { exited <- file.buf.String() })
			defer restore()
			gotest.Go(it, func() { gotestrunner.Exit(130, nil) })
			select {
			case <-exited:
				gotest.Fail(it, "the process was exited while the event was on its way to the file")
			case <-time.After(100 * time.Millisecond):
			}
			file.letGo()
			select {
			case atExit := <-exited:
				gotest.Equal(it, interruptedEvents, atExit)
			case <-time.After(5 * time.Second):
				gotest.Fail(it, "the process was not exited once the event was in the file")
			}
			wait()
			gotestrunner.ResumeWrites(gotestrunner.ExportExitGate)
		})
	})
	t.When("nothing is on its way", func(w *gotest.T) {
		w.It("returns at once and no event is written after it", func(it *gotest.T) {
			var stdout, file bytes.Buffer
			gate := &gotestrunner.WriteGate{}
			c := gotestrunner.NewOutputCollector(gotestrunner.RunStreamJSON, false,
				gotestrunner.WithWriters(&stdout, &bytes.Buffer{}), gotestrunner.WithEvents(&file), gotestrunner.WithWriteGate(gate))
			gotest.True(it, gotestrunner.EndWritesOf(gate, time.Minute, nil))
			wait := gotest.Go(it, func() { gotestrunner.WriteEvents(c, []byte(interruptedEvents)) })
			gotest.Consistently(it, 200*time.Millisecond, 20*time.Millisecond, func(poll *gotest.R) {
				gotest.Empty(poll, stdout.String())
				gotest.Empty(poll, file.String())
			})
			gotestrunner.ResumeWrites(gate)
			wait()
			gotest.Equal(it, interruptedEvents, stdout.String())
			gotest.Equal(it, stdout.String(), file.String())
		})
	})
}
