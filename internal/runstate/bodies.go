package runstate

import (
	"sync"
	"time"
)

// The bodies running now: test-method and benchmark bodies, fuzz executions
// and suites' BeforeAll hooks. A stop gives each grace to return, counted from
// the stop or, for one that starts after it, from its own start.
var (
	bodiesMu sync.Mutex
	bodies   = map[*body]struct{}{}
)

type body struct {
	owner any // the test it runs for
	kind  string
	start time.Time
}

// EnterBody records a body of kind ("a method", "BeforeAll", ...) running for
// owner, until the returned call.
func EnterBody(owner any, kind string) (exit func()) {
	b := &body{owner: owner, kind: kind, start: time.Now()}
	bodiesMu.Lock()
	bodies[b] = struct{}{}
	bodiesMu.Unlock()
	return func() {
		bodiesMu.Lock()
		delete(bodies, b)
		bodiesMu.Unlock()
	}
}

// Bodies reports how many bodies run now, leaving out those of except.
func Bodies(except any) int {
	bodiesMu.Lock()
	defer bodiesMu.Unlock()
	n := 0
	for b := range bodies {
		if except == nil || b.owner != except {
			n++
		}
	}
	return n
}

// Overdue names the kind of a body, other than except's, still running at now
// more than grace after the later of stopAt and its own start, or returns "".
func Overdue(stopAt time.Time, grace time.Duration, now time.Time, except any) string {
	bodiesMu.Lock()
	defer bodiesMu.Unlock()
	for b := range bodies {
		if except != nil && b.owner == except {
			continue
		}
		from := stopAt
		if b.start.After(from) {
			from = b.start
		}
		if now.Sub(from) > grace {
			return b.kind
		}
	}
	return ""
}
