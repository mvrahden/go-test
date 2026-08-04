package gotestruntime

import (
	"context"
	"errors"
	"fmt"
	"os"
	"runtime/debug"
	"time"

	"github.com/mvrahden/go-test/pkg/gotest"
)

// FixtureTeardown is one fixture's teardown policy: how long AfterAll may take
// and what it is held to by verdict.
//
// Both the in-process fixture DAG and the generated shared-fixture subprocess
// run AfterAll through this. They used to carry textually mirrored copies —
// the same drift RunFixtureSetup ended for setup — and neither copy held a
// teardown to its declared Timeout, so an AfterAll that ignored its context
// could overrun the budget its author wrote and still report success.
type FixtureTeardown struct {
	Name string
	// Timeout bounds the context AfterAll receives. Zero means unbounded.
	Timeout time.Duration
	// Budget is the deadline AfterAll is held to by verdict, or zero when the
	// fixture declared no config of its own.
	Budget time.Duration
	// AfterAll is the teardown to run. A nil AfterAll is "there is nothing to
	// release" and reports success.
	AfterAll func(ctx context.Context) error
}

// RunFixtureTeardown runs AfterAll under td's timeout policy, reports an
// error, a panic or a budget overrun on stderr, and returns whether teardown
// failed.
//
// A panic is contained rather than allowed to escape. Teardown runs
// concurrently across the fixture graph, so an unrecovered panic would abort
// the process from a goroutine and take every sibling's teardown down with it —
// the fixtures that had already started releasing resources would never finish.
func RunFixtureTeardown(ctx context.Context, td FixtureTeardown) (failed bool) {
	if td.AfterAll == nil {
		return false
	}
	defer func() {
		if r := recover(); r != nil {
			fmt.Fprintf(os.Stderr, "%s.AfterAll panicked: %v\n\n%s\n", td.Name, r, debug.Stack())
			failed = true
		}
	}()

	var cancel context.CancelFunc
	if td.Timeout > 0 {
		ctx, cancel = context.WithTimeout(ctx, td.Timeout)
	} else {
		ctx, cancel = context.WithCancel(ctx)
	}
	defer cancel()

	err := td.AfterAll(ctx)
	if err == nil && td.Budget > 0 && errors.Is(ctx.Err(), context.DeadlineExceeded) {
		// Teardown that ignores the context still overran the budget it was
		// given; without this a declared Timeout would bound setup by verdict
		// but teardown only by a context it is free to ignore. Only a declared
		// budget, though — failing a fixture against a default its author
		// never wrote is not a verdict they could act on.
		err = fmt.Errorf("exceeded its configured Timeout of %s", td.Budget)
	}
	if err != nil {
		fmt.Fprintf(os.Stderr, "%s.AfterAll failed: %v\n", td.Name, err)
		return true
	}
	return false
}

// SupervisorBudget is how long a fixture's own lifecycle stage may take, for
// the purpose of sizing the teardown budget handed to the supervisor. The
// generated shared-fixture subprocess reports its budget through this too — it
// used to carry a textual mirror, held in sync only by a generator test.
//
// Under literal config semantics a zero Timeout is the spelling of "no
// deadline", and a negative one is the documented "disabled" — neither means
// "takes no time". Reading them as zero would hand the supervisor a budget
// short enough to force-kill a teardown that is still releasing resources, and
// a signalled process reports no meaningful exit status, so the run would
// still be green. An unbounded fixture still needs headroom: fall back to the
// default floor.
func SupervisorBudget(timeout time.Duration) time.Duration {
	if timeout <= 0 {
		return gotest.DefaultFixtureConfig().Timeout
	}
	return timeout
}
