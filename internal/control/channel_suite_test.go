package control_test

import (
	"os"
	"os/exec"
	"strconv"
	"time"

	"github.com/mvrahden/go-test/internal/control"
	"github.com/mvrahden/go-test/internal/testkit"
	"github.com/mvrahden/go-test/pkg/gotest"
)

// ChannelTestSuite proves a channel carries lines both ways between the
// runner and a suite process, and that each side learns at once when the
// other is gone.
// Sequential: its methods time the channel's ends against a 100ms bound,
// which parallel children would stretch.
type ChannelTestSuite struct{}

// prompt bounds how soon a side learns the other is gone, or that its read
// was stopped.
var prompt = testkit.Scaled(100 * time.Millisecond)

func (s *ChannelTestSuite) BeforeEach(t *gotest.T) *children { return &children{} }

func (s *ChannelTestSuite) AfterEach(t *gotest.T, cs *children) { cs.reap() }

func (s *ChannelTestSuite) TestRoundTrip(t *gotest.T, cs *children) {
	c := cs.start(t, spec{mode: "roundtrip", link: true})
	hello, budget, stopped := c.next(t), c.next(t), c.next(t)
	stopErr, forceErr := c.link.Stop(), c.link.Force()
	first, second := c.value(t, "first"), c.value(t, "second")
	code := c.wait(t)

	t.It("delivers the child's lines parsed, and nothing it should not send", func(it *gotest.T) {
		gotest.Equal(it, control.Line{Kind: control.Hello, Version: control.Version, PID: c.cmd.Process.Pid}, hello)
		gotest.Equal(it, control.Line{Kind: control.Budget, Duration: 5*time.Minute + 30*time.Second}, budget)
		gotest.Equal(it, control.Line{Kind: control.Stopped, Reason: "signal:SIGTERM"}, stopped)
	})
	t.It("delivers stop, then force, to the child", func(it *gotest.T) {
		gotest.NoError(it, stopErr)
		gotest.NoError(it, forceErr)
		gotest.Equal(it, "stop", first)
		gotest.Equal(it, "force", second)
		gotest.Zero(it, code)
	})
	t.It("closes Lines once the child is gone", func(it *gotest.T) {
		gotest.True(it, c.closedWithin(within), "Lines stayed open after the child exited")
	})
}

func (s *ChannelTestSuite) TestRunnerSideAlone(t *gotest.T, cs *children) {
	t.When("the child's end closes without a process", func(w *gotest.T) {
		link, end := cs.pair(w)
		end.Close()
		end.Close()
		closed := (&child{link: link}).closedWithin(within)

		w.It("closes Lines", func(it *gotest.T) {
			gotest.True(it, closed)
		})
	})

	t.When("a line goes out longer after another than the send deadline", func(w *gotest.T) {
		link, _ := cs.pair(w)
		first := link.Stop()
		time.Sleep(testkit.Scaled(1200 * time.Millisecond))
		second := link.Force()

		w.It("sends both", func(it *gotest.T) {
			gotest.NoError(it, first)
			gotest.NoError(it, second)
		})
	})

	t.When("the link is closed while its reader waits", func(w *gotest.T) {
		link, end := cs.pair(w)
		time.Sleep(50 * time.Millisecond)
		returned := closeWithin(link, within)
		end.Close()
		link.Close()
		_, open := <-link.Lines()

		w.It("ends the read, closes Lines and refuses to send", func(it *gotest.T) {
			gotest.True(it, returned, "Close waited on the reader")
			gotest.False(it, open)
			gotest.Error(it, link.Stop())
			gotest.Error(it, link.Force())
		})
	})
}

// closeWithin reports whether link.Close returned within d.
func closeWithin(link *control.Link, d time.Duration) bool {
	done := make(chan struct{})
	go func() {
		link.Close()
		close(done)
	}()
	select {
	case <-done:
		return true
	case <-time.After(d):
		return false
	}
}

func (s *ChannelTestSuite) TestStop_NobodyReads(t *gotest.T, cs *children) {
	link, _ := cs.pair(t)
	gotest.NoError(t, link.Fill())
	start := time.Now()
	result := make(chan error, 1)
	go func() { result <- link.Stop() }()
	var err error
	returned := false
	select {
	case err = <-result:
		returned = true
	case <-time.After(within):
	}
	took := time.Since(start)

	t.It("gives up on the line within its deadline and says so", func(it *gotest.T) {
		gotest.True(it, returned, "Stop blocked on an end nobody reads")
		gotest.ErrorIs(it, err, os.ErrDeadlineExceeded)
		gotest.Less(it, took, testkit.Scaled(1500*time.Millisecond))
	})
}

func (s *ChannelTestSuite) TestAttach(t *gotest.T, cs *children) {
	t.When("the command has an environment naming a stale channel", func(w *gotest.T) {
		_, end := cs.pair(w)
		cmd := exec.Command("unused")
		cmd.Env = []string{"A=1", control.Env + "=stale"}
		prepareAttach(cmd)
		end.Attach(cmd)

		w.It("replaces the entry and keeps the others", func(it *gotest.T) {
			gotest.Len(it, cmd.Env, 2)
			gotest.Equal(it, "A=1", cmd.Env[0])
			checkAttached(it, cmd, cmd.Env[1], true)
		})
	})

	t.When("the command has no environment of its own", func(w *gotest.T) {
		_, end := cs.pair(w)
		cmd := exec.Command("unused")
		end.Attach(cmd)

		w.It("starts from this process's environment", func(it *gotest.T) {
			environ := environWithout(control.Env, os.Environ())
			gotest.Len(it, cmd.Env, len(environ)+1)
			gotest.Equal(it, environ, cmd.Env[:len(environ)])
			checkAttached(it, cmd, cmd.Env[len(environ)], false)
		})
	})
}

func (s *ChannelTestSuite) TestEnds(t *gotest.T, cs *children) {
	t.When("the runner kills the child", func(w *gotest.T) {
		c := cs.start(w, spec{mode: "wait", link: true})
		c.next(w)
		start := time.Now()
		gotest.NoError(w, c.cmd.Process.Kill())
		closed := c.closedWithin(prompt)
		took := time.Since(start)
		c.wait(w)

		w.It("closes Lines at once", func(it *gotest.T) {
			gotest.True(it, closed, "Lines stayed open %s after the kill", took)
		})
	})

	t.When("the runner closes its end", func(w *gotest.T) {
		c := cs.start(w, spec{mode: "wait", link: true})
		c.next(w)
		start := time.Now()
		c.link.Close()
		next := c.value(w, "next")
		took := time.Since(start)
		code := c.wait(w)

		w.It("ends the child's read at once", func(it *gotest.T) {
			gotest.Equal(it, "eof", next)
			gotest.Less(it, took, prompt)
			gotest.Zero(it, code)
		})
	})
}

func (s *ChannelTestSuite) TestNext_Stopped(t *gotest.T, cs *children) {
	c := cs.start(t, spec{mode: "deadline", link: true})
	c.next(t)
	first := c.value(t, "first")
	after, err := strconv.ParseInt(c.value(t, "first.after"), 10, 64)
	gotest.NoError(t, err)
	firstLeaked := c.value(t, "first.leaked")
	answer := c.next(t)
	gotest.NoError(t, c.link.Stop())
	second, secondLeaked := c.value(t, "second"), c.value(t, "second.leaked")
	code := c.wait(t)

	t.It("returns ErrStopped at once when stop closes, leaving no goroutine", func(it *gotest.T) {
		gotest.Equal(it, "stopped", first)
		gotest.Less(it, time.Duration(after), prompt)
		gotest.Equal(it, "false", firstLeaked)
	})
	t.It("reads the next line afterwards", func(it *gotest.T) {
		gotest.Equal(it, control.Line{Kind: control.Budget, Duration: time.Second}, answer)
		gotest.Equal(it, "stop", second)
		gotest.Equal(it, "false", secondLeaked)
		gotest.Zero(it, code)
	})
}

func (s *ChannelTestSuite) TestDrain(t *gotest.T, cs *children) {
	c := cs.start(t, spec{mode: "drain", link: true, stdin: true})
	before, beforeEOF := c.value(t, "before"), c.value(t, "before.eof")
	c.next(t)
	c.send(t, "busy")
	busy := c.value(t, "busy")
	busyTook, err := strconv.ParseInt(c.value(t, "busy.took"), 10, 64)
	gotest.NoError(t, err)
	gotest.NoError(t, c.link.WriteRaw([]byte("stop\nfor")))
	c.send(t, "whole")
	whole, wholeEOF := c.value(t, "whole"), c.value(t, "whole.eof")
	gotest.NoError(t, c.link.WriteRaw([]byte("ce\n")))
	c.send(t, "rest")
	rest, restEOF := c.value(t, "rest"), c.value(t, "rest.eof")
	c.link.Close()
	c.send(t, "closed")
	closed, closedEOF := c.value(t, "closed"), c.value(t, "closed.eof")
	_ = c.stdin.Close()
	code := c.wait(t)

	t.It("returns nothing before anything arrived", func(it *gotest.T) {
		gotest.Equal(it, "-", before)
		gotest.Equal(it, "false", beforeEOF)
	})
	t.It("returns at once while a Next reads", func(it *gotest.T) {
		gotest.Equal(it, "-", busy)
		gotest.Less(it, time.Duration(busyTook), prompt)
	})
	t.It("returns the whole lines that arrived and keeps a partial one", func(it *gotest.T) {
		gotest.Equal(it, "stop", whole)
		gotest.Equal(it, "false", wholeEOF)
		gotest.Equal(it, "force", rest)
		gotest.Equal(it, "false", restEOF)
	})
	t.It("reports the runner's end gone", func(it *gotest.T) {
		gotest.Equal(it, "-", closed)
		gotest.Equal(it, "true", closedEOF)
		gotest.Zero(it, code)
	})
}
