package gotestruntime

import (
	"fmt"
	"io"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/mvrahden/go-test/pkg/gotest"
)

// sharedTeardownHeadroom is what the shared fixture process adds to its
// fixtures' teardown budgets in the budget it reports.
const sharedTeardownHeadroom = 30 * time.Second

// orphanSetupBound stands in for the budget of a setup that declared no
// deadline, once nobody is left to wait for it.
const orphanSetupBound = 2 * time.Minute

// SharedLifetime bounds how long a shared fixture process lives on once its
// runner is gone: nobody is left to kill a setup or a teardown that hangs.
// The generated process records each fixture's setup and release on it.
type SharedLifetime struct {
	mu       sync.Mutex
	names    []string
	timeouts []time.Duration       // each fixture's Timeout; the default until its setup derives it
	inSetup  map[int]time.Duration // the setup budget of each setup running, zero for none declared
	begun    []bool
	released []bool
	reported time.Duration // the teardown budget told to the runner, once told

	stderr   io.Writer
	exit     func(int)
	sleep    func(time.Duration)
	headroom time.Duration
}

// NewSharedLifetime tracks the named fixtures, in the process's own order.
// It also keeps a write to a runner that is gone from ending the process, so
// a teardown that logs still runs to its end.
func NewSharedLifetime(names ...string) *SharedLifetime {
	notePipe()
	return newSharedLifetime(os.Stderr, os.Exit, time.Sleep, sharedTeardownHeadroom, names)
}

func newSharedLifetime(stderr io.Writer, exit func(int), sleep func(time.Duration), headroom time.Duration, names []string) *SharedLifetime {
	l := &SharedLifetime{
		names:    names,
		timeouts: make([]time.Duration, len(names)),
		inSetup:  map[int]time.Duration{},
		begun:    make([]bool, len(names)),
		released: make([]bool, len(names)),
		stderr:   stderr,
		exit:     exit,
		sleep:    sleep,
		headroom: headroom,
	}
	for i := range l.timeouts {
		l.timeouts[i] = gotest.DefaultFixtureConfig().Timeout
	}
	return l
}

// SetupBegins records that fixture i's setup starts under s.
func (l *SharedLifetime) SetupBegins(i int, s FixtureSetup) { //nolint:gocritic // hugeParam: the policy as the setup runs it
	l.mu.Lock()
	defer l.mu.Unlock()
	l.begun[i] = true
	l.timeouts[i] = s.Timeout
	l.inSetup[i] = SetupBudget(s)
}

// SetupEnds records that fixture i's setup has returned.
func (l *SharedLifetime) SetupEnds(i int) {
	l.mu.Lock()
	defer l.mu.Unlock()
	delete(l.inSetup, i)
}

// Released records that fixture i holds nothing any more.
func (l *SharedLifetime) Released(i int) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.released[i] = true
}

// Reported records the teardown budget the process told the runner.
func (l *SharedLifetime) Reported(teardown time.Duration) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.reported = teardown
}

// ExitWhenOrphaned waits for gone, closed once the runner is gone, then ends
// the process with status 1 when the process has had its budgets: the
// teardown budget, plus the setup budget of a setup still running, or
// orphanSetupBound for one that declared no deadline. It says on stderr
// which fixtures may have left something behind.
func (l *SharedLifetime) ExitWhenOrphaned(gone <-chan struct{}) {
	<-gone
	bound, phase, detail := l.bound()
	l.sleep(bound)
	l.mu.Lock()
	what := l.leftBehind()
	l.mu.Unlock()
	fmt.Fprintf(l.stderr, "gotest: the runner is gone and shared fixture %s did not finish within %s%s; %s may be left behind\n", phase, bound, detail, what)
	l.exit(1)
}

func (l *SharedLifetime) bound() (bound time.Duration, phase, detail string) {
	l.mu.Lock()
	defer l.mu.Unlock()
	teardown := l.reported
	if teardown <= 0 {
		for _, t := range l.timeouts {
			teardown += SupervisorBudget(t)
		}
		teardown += l.headroom
	}
	if len(l.inSetup) == 0 {
		return teardown, "teardown", ""
	}
	// The setups running are concurrent: a dependent one waits for its
	// dependency before it starts.
	var setup time.Duration
	unbounded := false
	for _, b := range l.inSetup {
		if b <= 0 {
			unbounded = true
		}
		setup = max(setup, b)
	}
	if unbounded {
		setup = max(setup, orphanSetupBound)
		return setup + teardown, "setup and teardown", fmt.Sprintf(" (a setup that declared no deadline was given a fixed %s, plus teardown budget %s)", setup, teardown)
	}
	return setup + teardown, "setup and teardown", fmt.Sprintf(" (setup budget %s plus teardown budget %s)", setup, teardown)
}

// leftBehind names what the fixtures whose setup began and that were not
// released yet hold.
func (l *SharedLifetime) leftBehind() string {
	var held []string
	for i, name := range l.names {
		if l.begun[i] && !l.released[i] {
			held = append(held, name)
		}
	}
	switch len(held) {
	case 0:
		return "what it holds"
	case 1:
		return "what " + held[0] + " holds"
	default:
		return "what " + strings.Join(held, ", ") + " hold"
	}
}
