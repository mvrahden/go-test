package gotestruntime

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/signal"
	"slices"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/mvrahden/go-test/internal/protocol/sharedwire"
	"github.com/mvrahden/go-test/pkg/gotest"
)

// SharedFixture is one shared fixture as the generated setup program
// describes it: what needs the fixture's own type stays there, as closures.
type SharedFixture struct {
	// Name is the type's name, for messages.
	Name string
	// Key is its state key: <import path>.<type name>.
	Key string
	// DependsOn holds the state keys of the fixtures it binds. A key that
	// names no fixture of the program is ignored.
	DependsOn []string
	// Deferred fixtures start on a start command, not up front.
	Deferred bool
	// Config is its SharedFixtureConfig; nil when it declares none.
	Config func() gotest.FixtureConfig
	// Wire assigns the fixtures it binds to its fields, once they are up;
	// nil when it binds none.
	Wire      func()
	BeforeAll func(ctx context.Context) error
	// AfterAll is nil when the fixture declares none.
	AfterAll func(ctx context.Context) error
	// State returns the value whose JSON is the fixture's state; nil for a
	// fixture with no transfer field.
	State func() any
}

// SharedProgram is the generated setup program's whole input.
type SharedProgram struct {
	Fixtures []SharedFixture
}

// RunSharedFixtures is the setup program's main. It returns once every
// fixture is released and all went well, and ends the process with status 1
// when a setup or a teardown failed.
func RunSharedFixtures(p SharedProgram) { //nolint:gocritic // hugeParam: a struct parameter, so a later field is an append
	// The runner is this process's parent: a parent pid that changed means
	// it is gone, and nobody waits for the teardown any more.
	parent := os.Getppid()
	// Shutdown-capable from birth: SIGTERM at any phase cancels setup, as the
	// runner's stop line does, so setup attempts abort instead of the default
	// disposition killing the process mid-lifecycle, and the epilogue still
	// releases whatever came up.
	signals, stop := signal.NotifyContext(context.Background(), syscall.SIGTERM, syscall.SIGINT)
	// Before any fixture code: its stdout is not the runner's to read.
	proto := ClaimSharedProtocol()
	life := NewSharedLifetime(fixtureNames(p.Fixtures)...)
	failed := runSharedProgram(p, sharedEnv{
		stdin:    os.Stdin,
		proto:    proto,
		signals:  signals,
		orphaned: func() bool { return os.Getppid() != parent },
		life:     life,
	})
	stop()
	// A clean exit is the runner's only proof that teardown ran and passed.
	if failed {
		os.Exit(sharedwire.ExitLifecycleFailed)
	}
}

func fixtureNames(fixtures []SharedFixture) []string {
	names := make([]string, len(fixtures))
	for i := range fixtures {
		names[i] = fixtures[i].Name
	}
	return names
}

// sharedEnv is what the program takes from its process.
type sharedEnv struct {
	stdin    io.Reader       // the runner's commands
	proto    io.Writer       // the protocol stream
	signals  context.Context // ends at SIGTERM or SIGINT
	orphaned func() bool     // whether the parent changed since the start
	life     *SharedLifetime
}

// sharedRun is one run of the program. Per-fixture slices are indexed by the
// fixture's position in the program.
type sharedRun struct {
	fx  []SharedFixture
	env sharedEnv

	ctx        context.Context // setup's: ends at a stop, a signal or the end of stdin
	releaseCtx context.Context // the releases': only a force ends it

	dependencies [][]int
	dependents   [][]int

	// Configs start as the defaults and are derived inside each fixture's
	// setup: a config marker that panics before the handshake would kill the
	// process attributed to nothing. A fixture whose derivation fails reports
	// the error, its dependents and its teardown skip it, and the defaults it
	// keeps are never read again.
	cfg    []gotest.FixtureConfig
	budget []time.Duration // the declared Timeout; zero without a config
	errs   []error
	done   []chan struct{}

	// Starts and teardowns run on goroutines of their own, so the flags
	// saying which fixtures came up, held resources and were released are
	// read and written under one lock.
	stateMu   sync.Mutex
	started   []bool
	attempted []bool
	torn      []bool

	// The teardown budget covers every fixture whose setup was reached, at
	// the config it derived: an up-front fixture by _done, a late one by the
	// acknowledgement of the start that brought it up. A late fixture not yet
	// started counts at the default config, so a stop during its start still
	// gets a window for its release. Teardowns run concurrently, each after
	// its dependents, so the budget is 30s plus the longest chain: a
	// fixture's share plus the longest chain among the fixtures depending on
	// it. A fixture's share leaves the budget once its teardown has run, and
	// an up-front fixture skipped for a failed dependency has nothing to
	// release.
	shareMu sync.Mutex
	shares  []time.Duration

	tornDone   []chan struct{}
	tornOnce   []sync.Once
	tornFailed []bool

	failMu         sync.Mutex
	teardownFailed bool

	protoMu sync.Mutex
}

func runSharedProgram(p SharedProgram, env sharedEnv) (failed bool) { //nolint:gocritic // hugeParam: as RunSharedFixtures
	n := len(p.Fixtures)
	r := &sharedRun{
		fx:         p.Fixtures,
		env:        env,
		cfg:        make([]gotest.FixtureConfig, n),
		budget:     make([]time.Duration, n),
		errs:       make([]error, n),
		done:       make([]chan struct{}, n),
		started:    make([]bool, n),
		attempted:  make([]bool, n),
		torn:       make([]bool, n),
		shares:     make([]time.Duration, n),
		tornDone:   make([]chan struct{}, n),
		tornOnce:   make([]sync.Once, n),
		tornFailed: make([]bool, n),
	}
	index := make(map[string]int, n)
	for i := range p.Fixtures {
		index[p.Fixtures[i].Key] = i
	}
	r.dependencies = make([][]int, n)
	r.dependents = make([][]int, n)
	for i := range p.Fixtures {
		for _, key := range p.Fixtures[i].DependsOn {
			if d, ok := index[key]; ok {
				r.dependencies[i] = append(r.dependencies[i], d)
				r.dependents[d] = append(r.dependents[d], i)
			}
		}
	}
	for i := range p.Fixtures {
		r.cfg[i] = WithFixtureDefaults(gotest.FixtureConfig{})
		r.done[i] = make(chan struct{})
		r.tornDone[i] = make(chan struct{})
		if p.Fixtures[i].Deferred {
			r.shares[i] = SupervisorBudget(r.cfg[i].Timeout)
		}
	}

	// The context every AfterAll runs under: only a force line ends it, so a
	// release that honours its context returns at once. A stop, by line or
	// by signal, leaves it alone.
	releaseCtx, force := context.WithCancel(context.Background())
	defer force()
	r.releaseCtx = releaseCtx
	ctx, end := context.WithCancel(env.signals)
	defer end()
	r.ctx = ctx

	cmds, eof := r.readCommands(end, force)

	// The runner is gone once stdin ends, or once a signal finds the parent
	// changed. From then on, in setup or in teardown, the process exits by
	// itself once it has had its budgets: with no runner left to kill it, a
	// BeforeAll or an AfterAll that hangs would otherwise keep it alive for
	// good.
	gone := make(chan struct{})
	go func() {
		defer close(gone)
		select {
		case <-eof:
		case <-ctx.Done():
			if !env.orphaned() {
				<-eof
			}
		}
	}()
	go env.life.ExitWhenOrphaned(gone)

	// Up-front phase: every non-deferred fixture, concurrently, gated by the
	// dependency channels. Deferred fixtures wait for their start command.
	var wg sync.WaitGroup
	for i := range r.fx {
		if r.fx[i].Deferred {
			continue
		}
		wg.Go(func() { r.setup(i) })
	}
	wg.Wait()

	anyFailed := slices.ContainsFunc(r.errs, func(err error) bool { return err != nil })
	// Reported on both _done forms: a failed setup still has succeeded
	// siblings to release under this budget.
	setupErr := ""
	if anyFailed {
		setupErr = sharedwire.SetupFailed
	}
	r.write(sharedwire.DoneLine(setupErr, r.teardownBudget()))

	// Window commands until shutdown. The runner owns shutdown timing, only
	// it knows when every suite has stopped using the fixtures, so even a
	// failed setup reports and then serves commands until the stop instead of
	// tearing down on its own; this process owns shutdown execution. Commands
	// run concurrently, so a release that hangs holds up no later start. A
	// stop that already arrived, by line or by SIGTERM (cancelling setup
	// above), has ctx closed and falls straight through, as does the end of
	// stdin. Starts still in flight when the loop ends are waited for, so no
	// AfterAll runs beside its own BeforeAll.
	var startsInFlight sync.WaitGroup
loop:
	for {
		select {
		case <-ctx.Done():
			break loop
		case <-eof:
			break loop
		case c := <-cmds:
			if c.Verb == sharedwire.VerbStart {
				startsInFlight.Add(1)
			}
			go func() {
				if c.Verb == sharedwire.VerbStart {
					defer startsInFlight.Done()
				}
				r.serve(c)
			}()
		}
	}
	startsInFlight.Wait()

	// Terminal teardown: everything still resident, concurrently, each after
	// its dependents. Keys a release command already released return its
	// outcome without running again: one owner per fixture teardown.
	var releasing sync.WaitGroup
	for i := len(r.fx) - 1; i >= 0; i-- {
		releasing.Go(func() { r.teardown(i) })
	}
	releasing.Wait()
	var tornErrs []string
	for i := len(r.fx) - 1; i >= 0; i-- {
		if r.teardown(i) {
			r.failed()
			tornErrs = append(tornErrs, r.fx[i].Name+": teardown failed")
		}
	}
	// The fixtures whose teardown failed, on the protocol stream: the
	// runner's teardown verdict when a setup failure has claimed the exit
	// status, never the fixtures' stderr, which carries whatever they write.
	if len(tornErrs) > 0 {
		r.write(sharedwire.TeardownLine(strings.Join(tornErrs, "; ")))
	}
	r.failMu.Lock()
	defer r.failMu.Unlock()
	// A failure of either kind; a start's failure was reported on its _cmd
	// line and is the runner's to book, not this status's.
	return anyFailed || r.teardownFailed
}

// readCommands reads the runner's commands from stdin, from the start. A stop
// line ends setup as SIGTERM does; a force line ends the releases' context
// too, and stops the process first when no stop is under way. Both are served
// here, not through cmds, whose send is dropped once setup's context is done.
// Stdin ending means the runner is gone, the one sign of it where no death
// signal is sent: it ends setup as SIGTERM does. The reader keeps reading past
// a shutdown, so a force line and the end of stdin are seen during teardown as
// well. A line is read whole however long it is: a start names every key it
// brings up, and a line cut short would end the reader as if the runner were
// gone.
func (r *sharedRun) readCommands(end, force context.CancelFunc) (cmds <-chan sharedwire.Command, eof <-chan struct{}) {
	commands := make(chan sharedwire.Command)
	ended := make(chan struct{})
	go func() {
		defer end()
		defer close(ended)
		in := bufio.NewReader(r.env.stdin)
		for {
			line, err := in.ReadBytes('\n')
			if err != nil && len(line) == 0 {
				return
			}
			c, ok := sharedwire.ParseCommand(line)
			if !ok {
				continue
			}
			switch c.Verb {
			case sharedwire.VerbStop:
				end()
			case sharedwire.VerbForce:
				force()
				end()
			default:
				select {
				case commands <- c:
				case <-r.ctx.Done():
				}
			}
		}
	}()
	return commands, ended
}

// write puts one line on the protocol stream, whole.
func (r *sharedRun) write(line []byte) {
	r.protoMu.Lock()
	defer r.protoMu.Unlock()
	_, _ = r.env.proto.Write(append(line, '\n'))
}

func (r *sharedRun) failed() {
	r.failMu.Lock()
	defer r.failMu.Unlock()
	r.teardownFailed = true
}

func (r *sharedRun) share(i int, timeout time.Duration) {
	r.shareMu.Lock()
	defer r.shareMu.Unlock()
	r.shares[i] = SupervisorBudget(timeout)
}

func (r *sharedRun) unshare(i int) {
	r.shareMu.Lock()
	defer r.shareMu.Unlock()
	r.shares[i] = 0
}

// teardownBudget is 30s plus the longest chain still resident, told to the
// lifetime too.
func (r *sharedRun) teardownBudget() time.Duration {
	r.shareMu.Lock()
	defer r.shareMu.Unlock()
	// Each chain once, whatever order the fixtures are listed in: the
	// dependencies form no cycle, so the recursion ends.
	chains := make([]time.Duration, len(r.shares))
	known := make([]bool, len(r.shares))
	var chain func(int) time.Duration
	chain = func(i int) time.Duration {
		if !known[i] {
			after := time.Duration(0)
			for _, d := range r.dependents[i] {
				after = max(after, chain(d))
			}
			chains[i], known[i] = r.shares[i]+after, true
		}
		return chains[i]
	}
	longest := time.Duration(0)
	for i := range r.shares {
		longest = max(longest, chain(i))
	}
	budget := sharedTeardownHeadroom + longest
	r.env.life.Reported(budget)
	return budget
}

// setup brings fixture i up, shared by the up-front phase and the start
// command: same config derivation, same dependency gating, same retry and
// budget policy through RunFixtureSetup. A fixture starts at most once.
func (r *sharedRun) setup(i int) {
	f := &r.fx[i]
	r.stateMu.Lock()
	first := !r.started[i]
	r.started[i] = true
	r.stateMu.Unlock()
	if !first {
		return
	}
	defer close(r.done[i])
	if f.Config != nil {
		// Derived before the dependency wait, on the bare instance: the same
		// moment the in-process harness derives it, before any parent fields
		// are assigned.
		cfg, err := DeriveFixtureConfig(f.Name, f.Config)
		if err != nil {
			r.errs[i] = err
			return
		}
		r.budget[i] = cfg.Timeout
		r.cfg[i] = WithFixtureDefaults(cfg)
	}
	for _, d := range r.dependencies[i] {
		<-r.done[d]
	}
	for _, d := range r.dependencies[i] {
		if r.errs[d] != nil {
			r.errs[i] = fmt.Errorf("skipped: dependency failed")
			return
		}
	}
	if f.Wire != nil {
		f.Wire()
	}
	// The budget the setup is about to be held to goes to the runner first,
	// whose wait for the whole setup follows what the fixtures declare unless
	// a --setup-timeout names a flat one. One policy, shared with the
	// in-process DAG: a panic here would otherwise kill the process holding
	// every sibling fixture's resources. RunFixtureSetup also writes the
	// per-attempt and final FAIL: lines to stderr. The fixture counts as
	// attempted, and so is released, only once its BeforeAll is entered and
	// until a release between attempts has run.
	s := FixtureSetup{
		Name:       f.Name,
		Timeout:    r.cfg[i].Timeout,
		Budget:     r.budget[i],
		Retries:    r.cfg[i].Retries,
		RetryDelay: r.cfg[i].RetryDelay,
		BeforeAll:  f.BeforeAll,
		AfterAll:   f.AfterAll,
		Held: func(held bool) {
			r.stateMu.Lock()
			defer r.stateMu.Unlock()
			r.attempted[i] = held
		},
	}
	r.share(i, s.Timeout)
	r.env.life.SetupBegins(i, s)
	r.write(sharedwire.SetupLine(f.Key, SetupBudget(s)))
	r.errs[i] = RunFixtureSetup(r.ctx, s)
	r.env.life.SetupEnds(i)
	if r.errs[i] != nil {
		return
	}
	var state []byte
	if f.State != nil {
		b, err := json.Marshal(f.State())
		if err != nil {
			r.errs[i] = fmt.Errorf("marshal: %w", err)
			fmt.Fprintf(os.Stderr, "%s: marshal: %v\n", f.Name, err)
			return
		}
		state = b
	}
	r.write(sharedwire.StateLine(f.Key, state))
}

// teardown releases fixture i and says whether that failed, shared by the
// release command and the epilogue: it runs once, whoever reaches it first,
// and a later caller gets its outcome. Teardowns run concurrently, each once
// every dependent holding resources has been released, or is being; a
// dependent whose BeforeAll never ran counts as released. A fixture whose
// BeforeAll never ran (never started, a config error, a dependency that
// failed) has nothing to release; one whose BeforeAll ran is released
// however it came out, since a failed attempt may hold what it reached.
func (r *sharedRun) teardown(i int) bool {
	r.tornOnce[i].Do(func() {
		defer close(r.tornDone[i])
		// A release sent while the fixture's start still runs waits for that
		// setup to end: no AfterAll beside its own BeforeAll.
		r.stateMu.Lock()
		begun := r.started[i]
		r.stateMu.Unlock()
		if begun {
			<-r.done[i]
		}
		for _, d := range r.dependents[i] {
			r.stateMu.Lock()
			held := r.attempted[d] || r.torn[d]
			r.stateMu.Unlock()
			if held {
				<-r.tornDone[d]
			}
		}
		r.stateMu.Lock()
		skip := r.torn[i] || !r.attempted[i]
		r.torn[i] = true
		r.stateMu.Unlock()
		defer r.env.life.Released(i)
		defer r.unshare(i)
		if skip {
			return
		}
		r.tornFailed[i] = RunFixtureTeardown(r.releaseCtx, FixtureTeardown{
			Name:     r.fx[i].Name,
			Timeout:  r.cfg[i].Timeout,
			Budget:   r.budget[i],
			AfterAll: r.fx[i].AfterAll,
		})
	})
	return r.tornFailed[i]
}

// serve runs one command and acknowledges it with its id, the joined
// per-fixture failures and the teardown budget, which now covers what the
// command brought up.
func (r *sharedRun) serve(c sharedwire.Command) { //nolint:gocritic // hugeParam: a command, read once
	var errs []string
	requested := func(i int) bool { return slices.Contains(c.Keys, r.fx[i].Key) }
	// A command that could only hang is refused whole, before anything
	// starts or is released: a start waiting for a dependency that is neither
	// up nor started with it, or a release waiting for a dependent that is up
	// and not released with it. So is a start of a fixture released earlier:
	// its state is gone, and a second BeforeAll would bring it up twice in
	// one run. releases: the keys this command releases; one released earlier
	// reports nothing again.
	releases := make([]bool, len(r.fx))
	r.stateMu.Lock()
	for i := range r.fx {
		if !requested(i) {
			continue
		}
		releases[i] = !r.torn[i]
		switch c.Verb {
		case sharedwire.VerbStart:
			if r.torn[i] {
				errs = append(errs, r.fx[i].Name+": released earlier in this run; shared fixtures are never restarted")
			}
			for _, d := range r.dependencies[i] {
				if !r.started[d] && !requested(d) {
					errs = append(errs, r.fx[i].Name+": its dependency "+r.fx[d].Name+" is neither up nor started with it")
				}
			}
		case sharedwire.VerbTeardown:
			for _, d := range r.dependents[i] {
				if r.attempted[d] && !r.torn[d] && !requested(d) {
					errs = append(errs, r.fx[i].Name+": "+r.fx[d].Name+", which depends on it, "+sharedwire.ReleaseRefused)
				}
			}
		}
	}
	r.stateMu.Unlock()
	switch {
	case len(errs) > 0:
	case c.Verb == sharedwire.VerbStart:
		// Every requested fixture starts on a goroutine of its own, as in the
		// up-front phase: the dependency channels order them, whatever order
		// the keys came in.
		var starting sync.WaitGroup
		for i := range r.fx {
			if requested(i) {
				starting.Go(func() { r.setup(i) })
			}
		}
		starting.Wait()
		for i := range r.fx {
			if requested(i) && r.errs[i] != nil {
				errs = append(errs, r.fx[i].Name+": "+r.errs[i].Error())
			}
		}
	case c.Verb == sharedwire.VerbTeardown:
		var releasing sync.WaitGroup
		for i := len(r.fx) - 1; i >= 0; i-- {
			if requested(i) {
				releasing.Go(func() { r.teardown(i) })
			}
		}
		releasing.Wait()
		for i := len(r.fx) - 1; i >= 0; i-- {
			if requested(i) && releases[i] && r.teardown(i) {
				r.failed()
				errs = append(errs, r.fx[i].Name+": teardown failed")
			}
		}
	default:
		errs = append(errs, "unknown verb: "+c.Verb)
	}
	r.write(sharedwire.AckLine(c.ID, strings.Join(errs, "; "), r.teardownBudget()))
}
