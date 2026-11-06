// Package control is the channel between the gotest runner and a suite
// process it starts. The runner creates a connected pair, the suite process
// inherits one end and finds it named in GOTEST_CONTROL, and the two sides
// exchange newline-terminated ASCII lines of at most 256 bytes:
//
//	hello 1 <pid>      up: the suite took the channel; 1 is the protocol version
//	budget <duration>  up: the teardown budget the suite asks for
//	stopped <reason>   up: a stop the runner did not send began
//	stop               down: the first stop
//	force              down: the second stop
//
// The end of the stream means the other side is gone. Unknown lines, lines
// sent the wrong way and longer lines are ignored.
package control

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

// Env names the variable that tells a suite process where its end is.
const Env = "GOTEST_CONTROL"

// Version is the protocol version a hello line carries.
const Version = 1

// MaxLine is the longest line in bytes, its newline included.
const MaxLine = 256

// Kind is the verb a line starts with.
type Kind uint8

const (
	_ Kind = iota
	Hello
	Budget
	Stopped
	Stop
	Force
)

var kindNames = [...]string{Hello: "hello", Budget: "budget", Stopped: "stopped", Stop: "stop", Force: "force"}

func (k Kind) String() string {
	if int(k) < len(kindNames) && kindNames[k] != "" {
		return kindNames[k]
	}
	return "Kind(" + strconv.Itoa(int(k)) + ")"
}

// up reports whether the suite process sends lines of this kind.
func (k Kind) up() bool { return k == Hello || k == Budget || k == Stopped }

// Line is one parsed line. Only the fields of its kind are set.
type Line struct {
	Kind     Kind
	Version  int           // hello
	PID      int           // hello
	Duration time.Duration // budget
	Reason   string        // stopped
}

// ErrStopped is what Next returns when its stop channel closed first.
var ErrStopped = errors.New("control: read stopped")

// parse reads one line without its newline. Every field must be present and
// nothing else; anything else is not a line.
func parse(b []byte) (Line, bool) {
	for _, c := range b {
		if c < ' ' || c > '~' {
			return Line{}, false
		}
	}
	f := strings.Split(string(b), " ")
	switch {
	case f[0] == "hello" && len(f) == 3:
		v, okV := decimal(f[1])
		pid, okP := decimal(f[2])
		if okV && okP && v > 0 && pid > 0 {
			return Line{Kind: Hello, Version: v, PID: pid}, true
		}
	case f[0] == "budget" && len(f) == 2:
		if d, err := time.ParseDuration(f[1]); err == nil && d >= 0 {
			return Line{Kind: Budget, Duration: d}, true
		}
	case f[0] == "stopped" && len(f) == 2 && f[1] != "":
		return Line{Kind: Stopped, Reason: f[1]}, true
	case f[0] == "stop" && len(f) == 1:
		return Line{Kind: Stop}, true
	case f[0] == "force" && len(f) == 1:
		return Line{Kind: Force}, true
	}
	return Line{}, false
}

// decimal parses digits only: no sign, no spaces.
func decimal(s string) (int, bool) {
	if s == "" {
		return 0, false
	}
	for i := range len(s) {
		if s[i] < '0' || s[i] > '9' {
			return 0, false
		}
	}
	n, err := strconv.Atoi(s)
	return n, err == nil
}

// format spells l as a line, newline included, and refuses one the other side
// would not read back as l.
func format(l Line) ([]byte, error) {
	var s string
	switch l.Kind {
	case Hello:
		s = fmt.Sprintf("hello %d %d", l.Version, l.PID)
	case Budget:
		s = "budget " + strings.Replace(l.Duration.String(), "µ", "u", 1)
	case Stopped:
		s = "stopped " + l.Reason
	case Stop, Force:
		s = l.Kind.String()
	}
	if back, ok := parse([]byte(s)); !ok || back != l || len(s) >= MaxLine {
		return nil, fmt.Errorf("control: %q is not a line", s)
	}
	return []byte(s + "\n"), nil
}

// lineReader splits a stream into lines. A partial line waits for the rest;
// a line longer than MaxLine is dropped up to its newline.
type lineReader struct {
	up   bool // keep the lines a suite process sends, else the runner's
	buf  []byte
	skip bool
}

func (r *lineReader) feed(p []byte, out []Line) []Line {
	for len(p) > 0 {
		i := bytes.IndexByte(p, '\n')
		if i < 0 {
			if !r.skip && len(r.buf)+len(p) < MaxLine {
				r.buf = append(r.buf, p...)
			} else {
				r.buf, r.skip = r.buf[:0], true
			}
			return out
		}
		chunk := p[:i]
		p = p[i+1:]
		if !r.skip && len(r.buf)+len(chunk) < MaxLine {
			r.buf = append(r.buf, chunk...)
			if l, ok := parse(r.buf); ok && l.Kind.up() == r.up {
				out = append(out, l)
			}
		}
		r.buf, r.skip = r.buf[:0], false
	}
	return out
}

// Link is the runner's end of one suite process's channel.
type Link struct {
	r, w   *os.File // the same file where one descriptor carries both ways
	lines  chan Line
	done   chan struct{}
	exited chan struct{}
	once   sync.Once
}

func newLink(r, w *os.File) *Link {
	l := &Link{r: r, w: w, lines: make(chan Line, 8), done: make(chan struct{}), exited: make(chan struct{})}
	go l.read()
	return l
}

func (l *Link) read() {
	defer close(l.exited)
	defer close(l.lines)
	lr := lineReader{up: true}
	buf := make([]byte, 512)
	var lines []Line
	for {
		n, err := l.r.Read(buf)
		lines = lr.feed(buf[:n], lines[:0])
		for _, line := range lines {
			select {
			case l.lines <- line:
			case <-l.done:
				return
			}
		}
		if err != nil {
			return
		}
	}
}

// Lines delivers the hello, budget and stopped lines as they arrive. It is
// closed once the suite's side is gone, or the Link is closed.
func (l *Link) Lines() <-chan Line { return l.lines }

// sendWithin bounds a write to the suite process: one that holds its end
// without reading cannot block the runner.
const sendWithin = time.Second

// Stop asks the suite process to stop. An error, a missed deadline included,
// means the line did not go out.
func (l *Link) Stop() error { return l.send(Line{Kind: Stop}) }

// Force asks the suite process to release what it holds at once.
func (l *Link) Force() error { return l.send(Line{Kind: Force}) }

func (l *Link) send(line Line) error {
	b, err := format(line)
	if err != nil {
		return err
	}
	if err := l.w.SetWriteDeadline(time.Now().Add(sendWithin)); err != nil {
		return err
	}
	_, err = l.w.Write(b)
	_ = l.w.SetWriteDeadline(time.Time{})
	return err
}

// Close closes the runner's end; the suite process reads the end of the
// stream. It is safe to call more than once.
func (l *Link) Close() {
	l.once.Do(func() {
		close(l.done)
		_ = l.r.Close()
		if l.w != l.r {
			_ = l.w.Close()
		}
		<-l.exited
	})
}

// setEnv names value in cmd's environment, replacing an earlier entry.
func setEnv(cmd *exec.Cmd, value string) {
	env := cmd.Env
	if env == nil {
		env = os.Environ()
	}
	kept := make([]string, 0, len(env)+1)
	for _, kv := range env {
		if k, _, _ := strings.Cut(kv, "="); k != Env && (runtime.GOOS != "windows" || !strings.EqualFold(k, Env)) {
			kept = append(kept, kv)
		}
	}
	kept = append(kept, Env+"="+value)
	cmd.Env = kept
}

// Endpoint is a suite process's end of the channel.
type Endpoint struct {
	r, w *os.File // the same file where one descriptor carries both ways

	mu      sync.Mutex // held by a reading Next, so Drain can tell
	lr      lineReader
	pending []Line
	buf     []byte
	eof     atomic.Bool
	once    sync.Once
}

// Claim takes the channel the runner passed and removes its variable from
// the environment. It returns nil when there is none, as under plain go test,
// or when the descriptor named is not one, which it then leaves alone.
func Claim() *Endpoint {
	v, ok := os.LookupEnv(Env)
	if !ok {
		return nil
	}
	_ = os.Unsetenv(Env)
	return claim(v)
}

// newEndpoint closes and refuses files whose reads cannot be ended through a
// deadline: Next could not be stopped on them.
func newEndpoint(r, w *os.File) *Endpoint {
	if r.SetReadDeadline(time.Time{}) != nil {
		_ = r.Close()
		if w != r {
			_ = w.Close()
		}
		return nil
	}
	return &Endpoint{r: r, w: w, lr: lineReader{up: false}, buf: make([]byte, 512)}
}

// Hello announces that the channel is taken.
func (e *Endpoint) Hello(pid int) error { return e.send(Line{Kind: Hello, Version: Version, PID: pid}) }

// Budget reports the teardown budget the process needs.
func (e *Endpoint) Budget(d time.Duration) error { return e.send(Line{Kind: Budget, Duration: d}) }

// Stopped reports a stop the runner did not send, such as "signal:SIGTERM".
// The reason is one word.
func (e *Endpoint) Stopped(reason string) error {
	return e.send(Line{Kind: Stopped, Reason: reason})
}

func (e *Endpoint) send(line Line) error {
	b, err := format(line)
	if err != nil {
		return err
	}
	_, err = e.w.Write(b)
	return err
}

// Drain returns the lines that arrived and were not read yet, and whether the
// runner's end is gone. It never blocks: while a Next is reading it returns
// nothing, as that Next delivers what arrives. A partial line is kept for
// the next call.
func (e *Endpoint) Drain() ([]Line, bool) {
	if !e.mu.TryLock() {
		return nil, e.eof.Load()
	}
	defer e.mu.Unlock()
	for !e.eof.Load() {
		n, eof := readNow(e.r, e.buf)
		e.pending = e.lr.feed(e.buf[:n], e.pending)
		if eof {
			e.eof.Store(true)
		}
		if n == 0 {
			break
		}
	}
	lines := e.pending
	e.pending = nil
	return lines, e.eof.Load()
}

// Next blocks for the next line. It returns io.EOF once the runner's end is
// gone, and ErrStopped once stop is closed; a line that arrives then is kept
// for the next call. It leaves no goroutine behind.
func (e *Endpoint) Next(stop <-chan struct{}) (Line, error) {
	e.mu.Lock()
	defer e.mu.Unlock()
	for {
		select {
		case <-stop:
			return Line{}, ErrStopped
		default:
		}
		if len(e.pending) > 0 {
			l := e.pending[0]
			e.pending = e.pending[1:]
			return l, nil
		}
		if e.eof.Load() {
			return Line{}, io.EOF
		}
		n, err := e.read(stop)
		e.pending = e.lr.feed(e.buf[:n], e.pending)
		// A reset by a runner that closed with lines unread is its end too.
		if err != nil && !errors.Is(err, os.ErrDeadlineExceeded) {
			e.eof.Store(true)
		}
	}
}

// read is one Read that a closed stop cuts short through the read deadline,
// which is cleared again before it returns.
func (e *Endpoint) read(stop <-chan struct{}) (int, error) {
	if stop == nil {
		return e.r.Read(e.buf)
	}
	done := make(chan struct{})
	exited := make(chan struct{})
	go func() {
		defer close(exited)
		select {
		case <-stop:
			_ = e.r.SetReadDeadline(time.Unix(1, 0))
		case <-done:
		}
	}()
	n, err := e.r.Read(e.buf)
	close(done)
	<-exited
	_ = e.r.SetReadDeadline(time.Time{})
	return n, err
}

// Close closes the process's end; the runner reads the end of the stream once
// no other process holds it.
func (e *Endpoint) Close() {
	e.once.Do(func() {
		_ = e.r.Close()
		if e.w != e.r {
			_ = e.w.Close()
		}
	})
}
