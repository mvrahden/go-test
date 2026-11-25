package gotestrunner

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"maps"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/mvrahden/go-test/internal/protocol"
)

// Exclusive scopes across runs.
//
// A scope's lock is a file under the gotest cache, keyed by module and
// group, so two runs of one module, from one checkout or from two, never
// run the same exclusive scope at once. A run holds one scope at a time,
// since its scopes never overlap, and takes a scope's modules in sorted
// order: two runs never wait for each other in a cycle. The OS drops a lock
// whose process died.

// scopeKey names what two runs' exclusive suites must share to exclude
// each other: the module, and the group, "" for the default group's
// Exclusive suites.
type scopeKey struct{ Module, Group string }

func (k scopeKey) String() string {
	if k.Group == "" {
		return "the exclusive suites of " + k.Module
	}
	return "group " + k.Group + " of " + k.Module
}

// exclusiveLockPath is the lock file of k under the cache root.
func exclusiveLockPath(root string, k scopeKey) string {
	sum := sha256.Sum256([]byte(k.Module + "\x00" + k.Group))
	return filepath.Join(root, "exclusive", hex.EncodeToString(sum[:8])+".lock")
}

// scopeHolder is what a holder leaves beside its lock for a run that waits.
type scopeHolder struct {
	PID   int       `json:"pid"`
	Since time.Time `json:"since"`
	Dir   string    `json:"dir"`
	Args  []string  `json:"args"`
}

func (h *scopeHolder) String() string {
	s := fmt.Sprintf("pid %d", h.PID)
	if h.Dir != "" {
		s += " in " + h.Dir
	}
	s += ", since " + h.Since.Local().Format("15:04:05")
	if len(h.Args) > 0 {
		s += ": gotest " + strings.Join(h.Args, " ")
	}
	return s
}

// crossRunLocks takes a run's exclusive scopes across runs.
type crossRunLocks struct {
	root      string          // the cache root; "" when the lock is off
	inherited map[string]bool // lock files the run that started this one holds
	stderr    io.Writer

	mu sync.Mutex
	// held are the lock files this run holds now.
	held []string
	// cut names the scope a wait the run's end cut short was for.
	cut string
	// warned: a lock that could not be taken was reported.
	warned bool
}

// newCrossRunLocks reads the switch and what a parent run holds.
func newCrossRunLocks(stderr io.Writer) *crossRunLocks {
	l := &crossRunLocks{stderr: stderr, inherited: map[string]bool{}}
	if os.Getenv(protocol.EnvExclusiveLock) == "0" {
		return l
	}
	if root, err := cacheRoot(); err == nil {
		l.root = root
	}
	for _, p := range filepath.SplitList(os.Getenv(protocol.EnvExclusiveHeld)) {
		if p != "" {
			l.inherited[p] = true
		}
	}
	return l
}

// withHeld adds the lock files this run holds, and those it inherited, to
// a suite's environment: a gotest run the suite starts takes them as held.
func (l *crossRunLocks) withHeld(env []string) []string {
	l.mu.Lock()
	defer l.mu.Unlock()
	if len(l.held) == 0 {
		return env
	}
	paths := slices.Concat(l.held, slices.Sorted(maps.Keys(l.inherited)))
	return append(slices.Clone(env), protocol.EnvExclusiveHeld+"="+strings.Join(paths, string(os.PathListSeparator)))
}

// cutWait names the scope the run's end cut a wait short for, "" for none.
func (l *crossRunLocks) cutWait() string {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.cut
}

// acquire takes the locks of keys, sorted, and returns their release. A
// lock another run holds is waited for, said once on stderr; only ctx ends
// the wait, with its error. A lock that cannot be taken at all is reported
// once and passed over.
func (l *crossRunLocks) acquire(ctx context.Context, keys []scopeKey) (func(), error) {
	var releases []func()
	release := func() {
		for i := len(releases) - 1; i >= 0; i-- {
			releases[i]()
		}
	}
	if l.root == "" {
		return release, ctx.Err()
	}
	for _, k := range keys {
		if err := ctx.Err(); err != nil {
			release()
			return nil, err
		}
		path := exclusiveLockPath(l.root, k)
		if l.inherited[path] {
			continue
		}
		unlock, err := l.take(ctx, k, path)
		if err != nil {
			if ctx.Err() != nil {
				l.mu.Lock()
				l.cut = k.String()
				l.mu.Unlock()
				release()
				return nil, ctx.Err()
			}
			l.warn(k, err)
			continue
		}
		releases = append(releases, unlock)
	}
	return release, nil
}

// take locks path, waiting while another run holds it, and leaves this
// run's holder record beside it.
func (l *crossRunLocks) take(ctx context.Context, k scopeKey, path string) (func(), error) {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return nil, err
	}
	unlock, err := tryLockFile(path)
	if err != nil {
		msg := fmt.Sprintf("gotest: %s waits for another gotest run", k)
		if h := readHolder(path); h != nil {
			msg += " (" + h.String() + ")"
		}
		fmt.Fprintln(l.stderr, msg)
		if unlock, err = waitLockFile(ctx, path); err != nil {
			return nil, err
		}
	}
	writeHolder(path)
	l.mu.Lock()
	l.held = append(l.held, path)
	l.mu.Unlock()
	return func() {
		l.mu.Lock()
		l.held = slices.DeleteFunc(l.held, func(p string) bool { return p == path })
		l.mu.Unlock()
		_ = os.Remove(path + ".holder")
		unlock()
	}, nil
}

func (l *crossRunLocks) warn(k scopeKey, err error) {
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.warned {
		return
	}
	l.warned = true
	fmt.Fprintf(l.stderr, "WARN: %s run without the lock that keeps other gotest runs out: %v\n", k, err)
}

// waitLockFile is lockFile until ctx ends. A lock taken after that is
// released at once.
func waitLockFile(ctx context.Context, path string) (func(), error) {
	type got struct {
		unlock func()
		err    error
	}
	ch := make(chan got, 1)
	go func() {
		unlock, err := lockFile(path)
		ch <- got{unlock, err}
	}()
	select {
	case g := <-ch:
		return g.unlock, g.err
	case <-ctx.Done():
		go func() {
			if g := <-ch; g.unlock != nil {
				g.unlock()
			}
		}()
		return nil, ctx.Err()
	}
}

func writeHolder(path string) {
	dir, _ := os.Getwd()
	b, err := json.Marshal(scopeHolder{PID: os.Getpid(), Since: time.Now(), Dir: dir, Args: os.Args[1:]})
	if err == nil {
		_ = os.WriteFile(path+".holder", b, 0o600)
	}
}

// readHolder is the record the holder of path left, nil when unreadable.
func readHolder(path string) *scopeHolder {
	b, err := os.ReadFile(path + ".holder")
	if err != nil {
		return nil
	}
	var h scopeHolder
	if json.Unmarshal(b, &h) != nil || h.PID == 0 {
		return nil
	}
	return &h
}

// scopeHold is one scope's hold across runs: taken by the first suite of
// the scope that needs it, released once the scope is over.
type scopeHold struct {
	locks *crossRunLocks
	keys  []scopeKey

	mu      sync.Mutex
	taken   bool
	release func()
}

// take holds the scope; idempotent. It fails only with ctx's error.
func (h *scopeHold) take(ctx context.Context) error {
	if h == nil {
		return nil
	}
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.taken {
		return nil
	}
	release, err := h.locks.acquire(ctx, h.keys)
	if err != nil {
		return err
	}
	h.taken, h.release = true, release
	return nil
}

// done releases the scope.
func (h *scopeHold) done() {
	if h == nil {
		return
	}
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.taken {
		h.release()
		h.taken, h.release = false, nil
	}
}

// planScopeHolds is the hold of each scope, nil for one that needs none: a
// named group holds its modules for the whole scope, the default group for
// its Exclusive suites.
func planScopeHolds(locks *crossRunLocks, scopes []suiteScope) []*scopeHold {
	holds := make([]*scopeHold, len(scopes))
	for i := range scopes {
		sc := &scopes[i]
		suites := flatten(sc.Tail)
		if sc.Group != "" {
			suites = sc.suites()
		}
		var keys []scopeKey
		for _, s := range suites {
			k := scopeKey{Module: s.Module, Group: sc.Group}
			if !slices.Contains(keys, k) {
				keys = append(keys, k)
			}
		}
		if len(keys) == 0 {
			continue
		}
		slices.SortFunc(keys, func(a, b scopeKey) int { return strings.Compare(a.Module, b.Module) })
		holds[i] = &scopeHold{locks: locks, keys: keys}
	}
	return holds
}

// holdsFor reports whether t runs under its scope's hold.
func holdsFor(t *SuiteTarget) bool { return t.Group != "" || t.Exclusive }
