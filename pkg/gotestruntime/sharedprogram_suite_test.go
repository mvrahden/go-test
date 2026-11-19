package gotestruntime_test

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/mvrahden/go-test/internal/protocol/sharedwire"
	"github.com/mvrahden/go-test/pkg/gotest"
	"github.com/mvrahden/go-test/pkg/gotestruntime"
)

// SharedProgramTestSuite: the shared fixture setup program, run in this
// process with its stdin and protocol stream in the test's hands.
type SharedProgramTestSuite struct{}

func (s *SharedProgramTestSuite) SuiteConfig() gotest.SuiteConfig {
	cfg := gotest.DefaultSuiteConfig()
	cfg.Parallel = true
	return cfg
}

// wire runs a program in this process: lines go in on its stdin and come
// out of its protocol stream.
type wire struct {
	t      *gotest.T
	stdin  *io.PipeWriter
	lines  chan string
	failed chan bool
	cancel context.CancelFunc
}

func startWire(t *gotest.T, fixtures ...gotestruntime.SharedFixture) *wire {
	inR, inW := io.Pipe()
	outR, outW := io.Pipe()
	signals, cancel := context.WithCancel(context.Background())
	w := &wire{t: t, stdin: inW, lines: make(chan string, 64), failed: make(chan bool, 1), cancel: cancel}
	go func() {
		defer close(w.lines)
		r := bufio.NewReader(outR)
		for {
			line, err := r.ReadString('\n')
			if line != "" {
				w.lines <- strings.TrimSuffix(line, "\n")
			}
			if err != nil {
				return
			}
		}
	}()
	go func() {
		failed := gotestruntime.ExportRunSharedProgram(gotestruntime.SharedProgram{Fixtures: fixtures}, inR, outW, signals)
		_ = outW.Close()
		w.failed <- failed
	}()
	return w
}

// send writes the line and a newline to the program's stdin.
func (w *wire) send(line string) {
	_, err := io.WriteString(w.stdin, line+"\n")
	gotest.NoError(w.t, err)
}

// next is the next protocol line.
func (w *wire) next() string {
	select {
	case line, ok := <-w.lines:
		if !ok {
			gotest.Fail(w.t, "the protocol stream ended")
		}
		return line
	case <-time.After(10 * time.Second):
		gotest.Fail(w.t, "no protocol line within 10s")
	}
	return ""
}

// until is the protocol lines up to and with the first keyed key.
func (w *wire) until(key string) []string {
	var lines []string
	for {
		line := w.next()
		lines = append(lines, line)
		if l, ok := sharedwire.ParseLine([]byte(line)); ok && l.Key == key {
			return lines
		}
	}
}

// ack is the next acknowledgement, decoded.
func (w *wire) ack() sharedwire.Line {
	lines := w.until(sharedwire.KeyCmd)
	l, _ := sharedwire.ParseLine([]byte(lines[len(lines)-1]))
	return l
}

func (w *wire) eof()    { _ = w.stdin.Close() }
func (w *wire) signal() { w.cancel() }

// wait is whether the program failed, once it returned; stdin is closed
// then, as the runner's ends with its process.
func (w *wire) wait() (failed bool) {
	defer w.eof()
	defer w.cancel()
	select {
	case failed = <-w.failed:
		return failed
	case <-time.After(10 * time.Second):
		gotest.Fail(w.t, "the program did not return within 10s")
	}
	return false
}

// returned says whether the program has returned, without waiting.
func (w *wire) returned() bool {
	select {
	case f := <-w.failed:
		w.failed <- f
		return true
	default:
		return false
	}
}

// rest is every protocol line left once the program returned.
func (w *wire) rest() []string {
	var lines []string
	for line := range w.lines {
		lines = append(lines, line)
	}
	return lines
}

// hookLog is what the fixtures' hooks did, in order.
type hookLog struct {
	mu     sync.Mutex
	events []string
}

func (l *hookLog) add(event string) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.events = append(l.events, event)
}

func (l *hookLog) all() []string {
	l.mu.Lock()
	defer l.mu.Unlock()
	return append([]string(nil), l.events...)
}

// fx is a fixture keyed "p."+name whose hooks log themselves and succeed.
func fx(log *hookLog, name string) gotestruntime.SharedFixture {
	return gotestruntime.SharedFixture{
		Name:      name,
		Key:       "p." + name,
		BeforeAll: func(context.Context) error { log.add(name + ".BeforeAll"); return nil },
		AfterAll:  func(context.Context) error { log.add(name + ".AfterAll"); return nil },
	}
}

func timeout(d time.Duration) func() gotest.FixtureConfig {
	return func() gotest.FixtureConfig {
		cfg := gotest.DefaultFixtureConfig()
		cfg.Timeout = d
		cfg.Retries = 0
		return cfg
	}
}

// lt is a JSON-escaped <, as encoding/json writes it.
const lt = "\x5cu003c"

func (s *SharedProgramTestSuite) TestUpFront(t *gotest.T) {
	two := gotestruntime.ScaleDuration(2 * time.Minute)
	log := &hookLog{}
	a := fx(log, "A")
	a.State = func() any {
		return struct {
			Addr any `json:"Addr"`
		}{"a<b"}
	}
	w := startWire(t, a)
	lines := []string{w.next(), w.next(), w.next()}
	w.send(`{"verb":"stop"}`)
	failed := w.wait()

	t.When("a fixture with one transfer field comes up", func(wh *gotest.T) {
		wh.It("reports its setup budget, its state and the end of setup, in that order", func(it *gotest.T) {
			gotest.Equal(it, []string{
				`{"key":"_setup","fixture":"p.A","budget":"` + two.String() + `"}`,
				`{"key":"p.A","state":{"Addr":"a` + lt + `b"}}`,
				`{"key":"_done","teardownBudget":"` + (30*time.Second + two).String() + `"}`,
			}, lines)
		})
		wh.It("ends clean on a stop line", func(it *gotest.T) {
			gotest.False(it, failed)
		})
		wh.It("released it once", func(it *gotest.T) {
			gotest.Equal(it, []string{"A.BeforeAll", "A.AfterAll"}, log.all())
		})
	})
}

func (s *SharedProgramTestSuite) TestNoTransferFields(t *gotest.T) {
	w := startWire(t, fx(&hookLog{}, "A"))
	_ = w.next()
	state := w.next()
	w.send(`{"verb":"stop"}`)
	w.wait()

	t.When("a fixture has no transfer field", func(wh *gotest.T) {
		wh.It("reports an empty state", func(it *gotest.T) {
			gotest.Equal(it, `{"key":"p.A","state":{}}`, state)
		})
	})
}

func (s *SharedProgramTestSuite) TestDependencyOrder(t *gotest.T) {
	log := &hookLog{}
	a, b := fx(log, "A"), fx(log, "B")
	b.DependsOn = []string{"p.A", "p.unknown"}
	b.Wire = func() { log.add("B.Wire") }
	w := startWire(t, a, b)
	lines := w.until(sharedwire.KeyDone)
	before := log.all()
	w.send(`{"verb":"stop"}`)
	w.wait()

	t.When("a fixture binds another", func(wh *gotest.T) {
		wh.It("starts it after its dependency is up, wired first", func(it *gotest.T) {
			gotest.Equal(it, []string{"A.BeforeAll", "B.Wire", "B.BeforeAll"}, before)
			gotest.Len(it, lines, 5)
			gotest.Regexp(it, `^\{"key":"_setup","fixture":"p\.A"`, lines[0])
			gotest.Equal(it, `{"key":"p.A","state":{}}`, lines[1])
			gotest.Regexp(it, `^\{"key":"_setup","fixture":"p\.B"`, lines[2])
			gotest.Equal(it, `{"key":"p.B","state":{}}`, lines[3])
		})
		wh.It("releases it before its dependency", func(it *gotest.T) {
			gotest.Equal(it, []string{"A.BeforeAll", "B.Wire", "B.BeforeAll", "B.AfterAll", "A.AfterAll"}, log.all())
		})
	})
}

func (s *SharedProgramTestSuite) TestAFailedDependency(t *gotest.T) {
	log := &hookLog{}
	a, b := fx(log, "A"), fx(log, "B")
	a.BeforeAll = func(context.Context) error { log.add("A.BeforeAll"); return errors.New("down") }
	b.DependsOn = []string{"p.A"}
	b.Wire = func() { log.add("B.Wire") }
	w := startWire(t, a, b)
	lines := w.until(sharedwire.KeyDone)
	w.send(`{"verb":"stop"}`)
	failed := w.wait()

	t.When("a fixture's dependency fails to come up", func(wh *gotest.T) {
		wh.It("never starts the dependent and reports the setup failed", func(it *gotest.T) {
			gotest.Len(it, lines, 2)
			gotest.Regexp(it, `^\{"key":"_setup","fixture":"p\.A"`, lines[0])
			gotest.Regexp(it, `^\{"key":"_done","error":"one or more shared fixtures failed","teardownBudget":"[^"]+"\}$`, lines[1])
		})
		wh.It("releases the dependency whose BeforeAll ran, and fails", func(it *gotest.T) {
			gotest.Equal(it, []string{"A.BeforeAll", "A.AfterAll"}, log.all())
			gotest.True(it, failed)
		})
	})
}

func (s *SharedProgramTestSuite) TestAConfigThatPanics(t *gotest.T) {
	log := &hookLog{}
	a, b := fx(log, "A"), fx(log, "B")
	a.Config = func() gotest.FixtureConfig { panic("no config") }
	w := startWire(t, a, b)
	lines := w.until(sharedwire.KeyDone)
	w.send(`{"verb":"stop"}`)
	failed := w.wait()

	t.When("a fixture's config panics", func(wh *gotest.T) {
		wh.It("fails the setup and leaves an independent fixture up", func(it *gotest.T) {
			gotest.Contains(it, lines, `{"key":"p.B","state":{}}`)
			gotest.Regexp(it, `^\{"key":"_done","error":"one or more shared fixtures failed"`, lines[len(lines)-1])
			gotest.True(it, failed)
		})
		wh.It("never runs the fixture's hooks", func(it *gotest.T) {
			gotest.Equal(it, []string{"B.BeforeAll", "B.AfterAll"}, log.all())
		})
	})
}

func (s *SharedProgramTestSuite) TestDeclaredBudgets(t *gotest.T) {
	sc := gotestruntime.ScaleDuration
	log := &hookLog{}
	two, three, ten := fx(log, "Two"), fx(log, "Three"), fx(log, "Ten")
	two.Config, three.Config, ten.Config = timeout(2*time.Second), timeout(3*time.Second), timeout(10*time.Second)
	three.DependsOn = []string{"p.Two"}
	w := startWire(t, two, three, ten)
	done := w.until(sharedwire.KeyDone)
	w.send(`{"verb":"teardown","keys":["p.Ten"],"id":1}`)
	ack := w.ack()
	w.send(`{"verb":"stop"}`)
	w.wait()

	t.When("fixtures declaring 2s, 3s on the 2s one, and 10s are up", func(wh *gotest.T) {
		wh.It("reports 30s plus the longest chain as the teardown budget", func(it *gotest.T) {
			gotest.Equal(it, `{"key":"_done","teardownBudget":"`+(30*time.Second+sc(10*time.Second)).String()+`"}`, done[len(done)-1])
		})
		wh.It("reports 30s plus the longest chain still resident once the 10s one is released", func(it *gotest.T) {
			gotest.Equal(it, "", ack.Error)
			gotest.Equal(it, (30*time.Second + sc(2*time.Second) + sc(3*time.Second)).String(), ack.TeardownBudget)
		})
	})
}

func (s *SharedProgramTestSuite) TestDeferred(t *gotest.T) {
	two := gotestruntime.ScaleDuration(2 * time.Minute)
	log := &hookLog{}
	a := fx(log, "A")
	a.Deferred = true
	w := startWire(t, a)
	first := w.next()
	before := log.all()
	w.send(`{"verb":"start","keys":["p.A"],"id":1}`)
	lines := []string{w.next(), w.next(), w.next()}
	w.send(`{"verb":"stop"}`)
	w.wait()

	t.When("a fixture is deferred", func(wh *gotest.T) {
		wh.It("ends the setup at once with its default share counted", func(it *gotest.T) {
			gotest.Equal(it, `{"key":"_done","teardownBudget":"`+(30*time.Second+two).String()+`"}`, first)
			gotest.Empty(it, before)
		})
		wh.It("starts it on a start command and acknowledges it with the teardown budget", func(it *gotest.T) {
			gotest.Equal(it, []string{
				`{"key":"_setup","fixture":"p.A","budget":"` + two.String() + `"}`,
				`{"key":"p.A","state":{}}`,
				`{"key":"_cmd","id":1,"error":"","teardownBudget":"` + (30*time.Second + two).String() + `"}`,
			}, lines)
		})
	})
}

func (s *SharedProgramTestSuite) TestCommandsRefusedWhole(t *gotest.T) {
	log := &hookLog{}
	a, b := fx(log, "A"), fx(log, "B")
	a.Deferred, b.Deferred = true, true
	b.DependsOn = []string{"p.A"}
	w := startWire(t, a, b)
	_ = w.until(sharedwire.KeyDone)
	w.send(`{"verb":"start","keys":["p.B"],"id":1}`)
	startAlone := w.ack()
	afterRefusal := log.all()
	w.send(`{"verb":"start","keys":["p.A","p.B"],"id":2}`)
	startBoth := w.ack()
	w.send(`{"verb":"teardown","keys":["p.A"],"id":3}`)
	releaseA := w.ack()
	afterRelease := log.all()
	w.send(`{"verb":"teardown","keys":["p.B"],"id":4}`)
	releaseB := w.ack()
	w.send(`{"verb":"start","keys":["p.B"],"id":5}`)
	restart := w.ack()
	w.send(`{"verb":"bogus","keys":["p.A"],"id":6}`)
	bogus := w.ack()
	w.send(`{"verb":"stop"}`)
	w.wait()

	t.When("a command could only hang or bring a fixture up twice", func(wh *gotest.T) {
		wh.It("refuses a start whose dependency is neither up nor started with it, starting nothing", func(it *gotest.T) {
			gotest.Equal(it, uint64(1), startAlone.ID)
			gotest.Equal(it, "B: its dependency A is neither up nor started with it", startAlone.Error)
			gotest.Empty(it, afterRefusal)
		})
		wh.It("starts a dependency and its dependent together", func(it *gotest.T) {
			gotest.Equal(it, uint64(2), startBoth.ID)
			gotest.Equal(it, "", startBoth.Error)
		})
		wh.It("refuses a release ahead of a dependent that is up, releasing nothing", func(it *gotest.T) {
			gotest.Equal(it, uint64(3), releaseA.ID)
			gotest.Equal(it, "A: B, which depends on it, is up and not released with it", releaseA.Error)
			gotest.Regexp(it, regexp.QuoteMeta(sharedwire.ReleaseRefused)+"$", releaseA.Error)
			gotest.Equal(it, []string{"A.BeforeAll", "B.BeforeAll"}, afterRelease)
		})
		wh.It("releases the dependent alone", func(it *gotest.T) {
			gotest.Equal(it, uint64(4), releaseB.ID)
			gotest.Equal(it, "", releaseB.Error)
		})
		wh.It("refuses to start a fixture released earlier", func(it *gotest.T) {
			gotest.Equal(it, uint64(5), restart.ID)
			gotest.Equal(it, "B: released earlier in this run; shared fixtures are never restarted", restart.Error)
		})
		wh.It("refuses a verb it does not know", func(it *gotest.T) {
			gotest.Equal(it, uint64(6), bogus.ID)
			gotest.Equal(it, "unknown verb: bogus", bogus.Error)
		})
	})
}

func (s *SharedProgramTestSuite) TestTwoFailuresAreJoinedInTheProgramsOrder(t *gotest.T) {
	log := &hookLog{}
	a, b := fx(log, "A"), fx(log, "B")
	for _, f := range []*gotestruntime.SharedFixture{&a, &b} {
		f.Deferred = true
		f.BeforeAll = func(context.Context) error { return errors.New("no") }
		f.AfterAll = func(context.Context) error { return errors.New("still no") }
	}
	w := startWire(t, a, b)
	_ = w.until(sharedwire.KeyDone)
	w.send(`{"verb":"start","keys":["p.B","p.A"],"id":1}`)
	start := w.ack()
	w.send(`{"verb":"stop"}`)
	failed := w.wait()
	rest := w.rest()

	t.When("two fixtures fail together", func(wh *gotest.T) {
		wh.It("joins their start failures in the program's order, whatever the keys' order", func(it *gotest.T) {
			gotest.Equal(it, "A: no; B: no", start.Error)
		})
		wh.It("joins their teardown failures in reverse order", func(it *gotest.T) {
			gotest.NotEmpty(it, rest)
			gotest.Equal(it, `{"key":"_teardown","error":"B: teardown failed; A: teardown failed"}`, rest[len(rest)-1])
			gotest.True(it, failed)
		})
	})
}

func (s *SharedProgramTestSuite) TestAStartIsServedOnce(t *gotest.T) {
	log := &hookLog{}
	a := fx(log, "A")
	a.Deferred = true
	w := startWire(t, a)
	_ = w.until(sharedwire.KeyDone)
	w.send(`{"verb":"start","keys":["p.A"],"id":1}`)
	first := w.ack()
	w.send(`{"verb":"start","keys":["p.A"],"id":2}`)
	second := w.ack()
	w.send(`{"verb":"stop"}`)
	w.wait()

	t.When("a fixture is started twice", func(wh *gotest.T) {
		wh.It("runs its BeforeAll once and acknowledges both", func(it *gotest.T) {
			gotest.Equal(it, []string{"A.BeforeAll", "A.AfterAll"}, log.all())
			gotest.Equal(it, "", first.Error)
			gotest.Equal(it, "", second.Error)
			gotest.Equal(it, uint64(2), second.ID)
		})
	})
}

// patient is a fixture whose AfterAll waits for its context or 5s, and says
// whether the context had ended when it returned.
func patient(entered chan<- struct{}, ended chan<- bool) gotestruntime.SharedFixture {
	return gotestruntime.SharedFixture{
		Name:      "A",
		Key:       "p.A",
		BeforeAll: func(context.Context) error { return nil },
		AfterAll: func(ctx context.Context) error {
			close(entered)
			select {
			case <-ctx.Done():
			case <-time.After(5 * time.Second):
			}
			ended <- ctx.Err() != nil
			return nil
		},
	}
}

func (s *SharedProgramTestSuite) TestStopForceAndTheEndOfStdin(t *gotest.T) {
	t.When("a stop line arrives and then a force line", func(wh *gotest.T) {
		entered, ended := make(chan struct{}), make(chan bool, 1)
		w := startWire(wh, patient(entered, ended))
		_ = w.until(sharedwire.KeyDone)
		w.send(`{"verb":"stop"}`)
		<-entered
		time.Sleep(200 * time.Millisecond)
		stillReleasing := !w.returned()
		w.send(`{"verb":"force"}`)
		failed := w.wait()
		wh.It("leaves the release to run after the stop, and ends its context at the force", func(it *gotest.T) {
			gotest.True(it, stillReleasing)
			gotest.True(it, <-ended)
			gotest.False(it, failed)
		})
	})
	t.When("a force line arrives alone", func(wh *gotest.T) {
		entered, ended := make(chan struct{}), make(chan bool, 1)
		w := startWire(wh, patient(entered, ended))
		_ = w.until(sharedwire.KeyDone)
		w.send(`{"verb":"force"}`)
		failed := w.wait()
		wh.It("stops the program with the release's context ended", func(it *gotest.T) {
			gotest.True(it, <-ended)
			gotest.False(it, failed)
		})
	})
	t.When("stdin ends", func(wh *gotest.T) {
		log := &hookLog{}
		w := startWire(wh, fx(log, "A"))
		_ = w.until(sharedwire.KeyDone)
		w.eof()
		failed := w.wait()
		wh.It("releases and returns clean", func(it *gotest.T) {
			gotest.Equal(it, []string{"A.BeforeAll", "A.AfterAll"}, log.all())
			gotest.False(it, failed)
		})
	})
	t.When("a stop signal arrives", func(wh *gotest.T) {
		log := &hookLog{}
		w := startWire(wh, fx(log, "A"))
		_ = w.until(sharedwire.KeyDone)
		w.signal()
		failed := w.wait()
		wh.It("releases and returns clean", func(it *gotest.T) {
			gotest.Equal(it, []string{"A.BeforeAll", "A.AfterAll"}, log.all())
			gotest.False(it, failed)
		})
	})
}

func (s *SharedProgramTestSuite) TestAStopDuringSetup(t *gotest.T) {
	log := &hookLog{}
	entered := make(chan struct{})
	a := fx(log, "A")
	a.BeforeAll = func(ctx context.Context) error {
		log.add("A.BeforeAll")
		close(entered)
		<-ctx.Done()
		return ctx.Err()
	}
	w := startWire(t, a)
	<-entered
	w.send(`{"verb":"stop"}`)
	lines := w.until(sharedwire.KeyDone)
	failed := w.wait()

	t.When("a stop arrives while a BeforeAll runs", func(wh *gotest.T) {
		wh.It("ends the setup through its context and reports it failed", func(it *gotest.T) {
			gotest.Regexp(it, `^\{"key":"_done","error":"one or more shared fixtures failed"`, lines[len(lines)-1])
			gotest.True(it, failed)
		})
		wh.It("releases the fixture once", func(it *gotest.T) {
			gotest.Equal(it, []string{"A.BeforeAll", "A.AfterAll"}, log.all())
		})
	})
}

func (s *SharedProgramTestSuite) TestAStateThatCannotBeMarshalled(t *gotest.T) {
	log := &hookLog{}
	a := fx(log, "A")
	a.State = func() any {
		return struct {
			C any `json:"C"`
		}{make(chan int)}
	}
	w := startWire(t, a)
	lines := w.until(sharedwire.KeyDone)
	w.send(`{"verb":"stop"}`)
	failed := w.wait()

	t.When("a fixture's state cannot be marshalled", func(wh *gotest.T) {
		wh.It("reports no state and fails the setup", func(it *gotest.T) {
			gotest.Len(it, lines, 2)
			gotest.Regexp(it, `^\{"key":"_done","error":"one or more shared fixtures failed"`, lines[1])
			gotest.True(it, failed)
		})
		wh.It("still releases it", func(it *gotest.T) {
			gotest.Equal(it, []string{"A.BeforeAll", "A.AfterAll"}, log.all())
		})
	})
}

func (s *SharedProgramTestSuite) TestALongCommand(t *gotest.T) {
	log := &hookLog{}
	a := fx(log, "A")
	a.Deferred = true
	w := startWire(t, a)
	_ = w.until(sharedwire.KeyDone)
	// A key list past 64 KiB: a start names every fixture it brings up.
	keys := []string{"p.A"}
	for i := range 4000 {
		keys = append(keys, fmt.Sprintf("p.not-in-this-program-%04d", i))
	}
	// Sent beside the test: a reader that gave up would never take it.
	long := string(sharedwire.Command{Verb: sharedwire.VerbStart, Keys: keys, ID: 1}.Line()) + "\n"
	go func() { _, _ = io.WriteString(w.stdin, long) }()
	lines := w.until(sharedwire.KeyCmd)
	ack, _ := sharedwire.ParseLine([]byte(lines[len(lines)-1]))
	w.send(`{"verb":"stop"}`)
	failed := w.wait()

	t.When("a command line is longer than 64 KiB", func(wh *gotest.T) {
		wh.It("is read whole and served", func(it *gotest.T) {
			gotest.Equal(it, uint64(1), ack.ID)
			gotest.Equal(it, "", ack.Error)
			gotest.Equal(it, []string{"A.BeforeAll", "A.AfterAll"}, log.all())
			gotest.False(it, failed)
		})
	})
}

func (s *SharedProgramTestSuite) TestLinesAreWrittenWhole(t *gotest.T) {
	big := strings.Repeat("x", 64<<10)
	var fixtures []gotestruntime.SharedFixture
	for i := range 8 {
		f := fx(&hookLog{}, fmt.Sprintf("F%d", i))
		f.State = func() any {
			return struct {
				Blob any `json:"Blob"`
			}{big}
		}
		fixtures = append(fixtures, f)
	}
	w := startWire(t, fixtures...)
	lines := w.until(sharedwire.KeyDone)
	w.send(`{"verb":"stop"}`)
	w.wait()

	t.When("eight fixtures report 64 KiB states at once", func(wh *gotest.T) {
		wh.It("writes every line whole", func(it *gotest.T) {
			gotest.Len(it, lines, 17)
			for _, line := range lines {
				_, ok := sharedwire.ParseLine([]byte(line))
				gotest.True(it, ok, "line of %d bytes", len(line))
			}
		})
	})
}
