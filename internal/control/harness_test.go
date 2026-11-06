package control_test

import (
	"bufio"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/mvrahden/go-test/internal/control"
	"github.com/mvrahden/go-test/internal/testkit"
	"github.com/mvrahden/go-test/pkg/gotest"
)

// within bounds one step of a child program: starting, answering, exiting.
var within = testkit.Scaled(10 * time.Second)

// children are the processes one test started, reaped after it.
type children struct {
	list   []*child
	probes []int
	links  []*control.Link
	ends   []*control.ChildEnd
}

// pair creates a channel no process uses.
func (cs *children) pair(t *gotest.T) (*control.Link, *control.ChildEnd) {
	link, end, err := control.New()
	gotest.NoError(t, err)
	cs.links, cs.ends = append(cs.links, link), append(cs.ends, end)
	return link, end
}

func (cs *children) reap() {
	for _, l := range cs.links {
		l.Close()
	}
	for _, e := range cs.ends {
		e.Close()
	}
	for _, c := range cs.list {
		if c.link != nil {
			c.link.Close()
		}
		if !c.waited {
			_ = c.cmd.Process.Kill()
			_ = c.cmd.Wait()
		}
		if c.stdin != nil {
			_ = c.stdin.Close()
		}
		_ = c.stderr.Close()
	}
	for _, pid := range cs.probes {
		if p, err := os.FindProcess(pid); err == nil {
			_ = p.Kill()
			_ = p.Release()
		}
	}
}

// spec says how to start a child program.
type spec struct {
	mode string
	link bool // create a channel and attach it
	// unnamed passes the channel without the variable naming it.
	unnamed bool
	value   *string // pass this as the channel instead
	file    bool    // pass a regular file as the channel instead
	stdin   bool
	// socket makes stdout a socket where the platform has one.
	socket bool
	env    []string
}

// child is a started child program and the runner's end of its channel.
type child struct {
	mode    string
	cmd     *exec.Cmd
	link    *control.Link
	channel string // what Attach named in GOTEST_CONTROL
	stdin   io.WriteCloser
	stderr  *os.File
	lines   chan string
	seen    map[string]string
	waited  bool
}

func (cs *children) start(t *gotest.T, s spec) *child {
	exe, err := os.Executable()
	gotest.NoError(t, err)
	cmd := exec.Command(exe) //nolint:gosec // G204: this test binary
	cmd.Env = append(baseEnv(s.mode), s.env...)
	stderr, err := os.CreateTemp(t.TempDir(), "stderr")
	gotest.NoError(t, err)
	cmd.Stderr = stderr
	c := &child{mode: s.mode, cmd: cmd, stderr: stderr, lines: make(chan string, 256), seen: map[string]string{}}

	var end *control.ChildEnd
	var file *os.File
	switch {
	case s.link:
		c.link, end, err = control.New()
		gotest.NoError(t, err)
		end.Attach(cmd)
		c.channel = moveChannel(t, cmd)
		if s.unnamed {
			cmd.Env = environWithout(envChannel, cmd.Env)
		}
	case s.value != nil:
		cmd.Env = append(cmd.Env, envChannel+"="+*s.value)
	case s.file:
		file, err = os.Create(filepath.Join(t.TempDir(), "file"))
		gotest.NoError(t, err)
		cmd.Env = append(cmd.Env, envChannel+"="+attachFile(cmd, file))
	}
	if s.stdin {
		c.stdin, err = cmd.StdinPipe()
		gotest.NoError(t, err)
	}
	var out io.Reader
	var stdoutEnd *os.File
	if s.socket {
		var r *os.File
		r, stdoutEnd, err = socketStdout(cmd)
		gotest.NoError(t, err)
		if r != nil {
			out = r
		}
	}
	if out == nil {
		out, err = cmd.StdoutPipe()
		gotest.NoError(t, err)
	}

	cs.list = append(cs.list, c)
	err = cmd.Start()
	// The child holds its own handle on the stderr file; the parent's would
	// keep Windows from removing the temp dir while it is still open.
	_ = stderr.Close()
	if end != nil {
		end.Close()
	}
	if stdoutEnd != nil {
		_ = stdoutEnd.Close()
	}
	if file != nil {
		_ = file.Close()
	}
	if err != nil {
		c.waited = true
		gotest.NoError(t, err)
	}
	go func() {
		sc := bufio.NewScanner(out)
		for sc.Scan() {
			c.lines <- sc.Text()
		}
		close(c.lines)
	}()
	return c
}

// moveChannel renames the entry Attach added, which must be the only one, to
// the name the child takes it under.
func moveChannel(t *gotest.T, cmd *exec.Cmd) string {
	var value string
	found := 0
	for i, kv := range cmd.Env {
		if k, v, _ := strings.Cut(kv, "="); k == control.Env {
			value = v
			found++
			cmd.Env[i] = envChannel + "=" + v
		}
	}
	gotest.Equal(t, 1, found, "Attach must name the channel once")
	return value
}

// value is what the child reported for key, read from its stdout as far as
// needed.
func (c *child) value(t *gotest.T, key string) string {
	if v, ok := c.seen[key]; ok {
		return v
	}
	timeout := time.After(within)
	for {
		select {
		case line, ok := <-c.lines:
			gotest.True(t, ok, "the %s child ended without reporting %s%s", c.mode, key, c.diagnosis())
			k, v, _ := strings.Cut(line, "=")
			c.seen[k] = v
			if k == key {
				return v
			}
		case <-timeout:
			gotest.Fail(t, "the %s child did not report %s within %s%s", c.mode, key, within, c.diagnosis())
		}
	}
}

// next is the next line the child sent.
func (c *child) next(t *gotest.T) control.Line {
	select {
	case l, ok := <-c.link.Lines():
		gotest.True(t, ok, "the %s child's channel ended before a line%s", c.mode, c.diagnosis())
		return l
	case <-time.After(within):
		gotest.Fail(t, "the %s child sent no line within %s%s", c.mode, within, c.diagnosis())
	}
	return control.Line{}
}

// closedWithin reports whether Lines closed within d, dropping lines meanwhile.
func (c *child) closedWithin(d time.Duration) bool {
	deadline := time.After(d)
	for {
		select {
		case _, ok := <-c.link.Lines():
			if !ok {
				return true
			}
		case <-deadline:
			return false
		}
	}
}

func (c *child) send(t *gotest.T, line string) {
	_, err := io.WriteString(c.stdin, line+"\n")
	gotest.NoError(t, err)
}

// wait waits for the child to exit and returns its exit code.
func (c *child) wait(t *gotest.T) int {
	done := make(chan error, 1)
	go func() { done <- c.cmd.Wait() }()
	select {
	case <-done:
	case <-time.After(within):
		_ = c.cmd.Process.Kill()
		<-done
		c.waited = true
		gotest.Fail(t, "the %s child did not exit within %s%s", c.mode, within, c.diagnosis())
	}
	c.waited = true
	return c.cmd.ProcessState.ExitCode()
}

// diagnosis is what the child wrote to stderr, for a failure message.
func (c *child) diagnosis() string {
	data, _ := os.ReadFile(c.stderr.Name())
	if len(data) == 0 {
		return ""
	}
	return "\nstderr:\n" + string(data)
}

// environWithout is env without key.
func environWithout(key string, env []string) []string {
	var kept []string
	for _, kv := range env {
		if k, _, _ := strings.Cut(kv, "="); k != key {
			kept = append(kept, kv)
		}
	}
	return kept
}
