package gotest

import (
	"context"
	"fmt"
	"iter"
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/mvrahden/go-test/pkg/gotest/internal/assert"
)

type TestCase func(*T)

func NewT(t *testing.T) *T {
	return &T{t: t}
}

type T struct {
	t         *testing.T
	ctx       context.Context
	collector *collectingT
}

func (t *T) T() *testing.T { return t.t }
func (t *T) Context() context.Context {
	if t.ctx != nil {
		return t.ctx
	}
	return t.t.Context()
}

func NewTWithDeadline(t *testing.T, timeout time.Duration) *T {
	ctx, cancel := context.WithTimeout(t.Context(), timeout)
	t.Cleanup(cancel)
	return &T{t: t, ctx: ctx}
}
func (t *T) Errorf(format string, args ...any) {
	if t.collector != nil {
		t.collector.Errorf(format, args...)
		return
	}
	t.t.Helper()
	t.t.Errorf(format, args...)
}
func (t *T) FailNow() {
	if t.collector != nil {
		t.collector.FailNow()
		return
	}
	t.t.FailNow()
}
//go:noinline
func execTestFn(testFn func(it *T), it *T) { testFn(it) }

func (t *T) It(description string, testFn func(it *T)) {
	t.t.Run(description, func(t *testing.T) {
		execTestFn(testFn, NewT(t))
	})
}
func (t *T) When(description string, fn func(w *T)) {
	t.t.Run(description, func(tt *testing.T) {
		execTestFn(fn, NewT(tt))
	})
}
func (t *T) Assert(v any) *assert.BaseAssertionContext {
	return assert.NewAssertionContext(v, t.t)
}

func (t *T) Each(entries any, fn any) {
	ev := reflect.ValueOf(entries)
	if ev.Kind() != reflect.Slice {
		t.t.Helper()
		t.t.Fatalf("Each: entries must be a slice, got %T", entries)
		return
	}

	fv := reflect.ValueOf(fn)
	ft := fv.Type()
	if ft.Kind() != reflect.Func || ft.NumIn() != 2 {
		t.t.Helper()
		t.t.Fatalf("Each: fn must be func(*gotest.T, EntryType), got %T", fn)
		return
	}

	for i := 0; i < ev.Len(); i++ {
		entry := ev.Index(i)
		name := eachEntryName(entry, i)
		t.t.Run(name, func(tt *testing.T) {
			fv.Call([]reflect.Value{reflect.ValueOf(NewT(tt)), entry})
		})
	}
}

func Each[E any](t *T, entries []E) iter.Seq2[*T, E] {
	return func(yield func(*T, E) bool) {
		for i, entry := range entries {
			name := eachEntryName(reflect.ValueOf(entry), i)
			t.t.Run(name, func(tt *testing.T) {
				yield(NewT(tt), entry)
			})
		}
	}
}

func eachEntryName(v reflect.Value, index int) string {
	if v.Kind() == reflect.Struct {
		for _, field := range []string{"Desc", "Name"} {
			f := v.FieldByName(field)
			if f.IsValid() && f.Kind() == reflect.String && f.String() != "" {
				return f.String()
			}
		}
	}
	return fmt.Sprintf("#%d", index)
}

// collectingT captures assertion failures without propagating them.
// Used by Eventually/Consistently to suppress intermediate poll failures.
type collectingT struct {
	failed  bool
	message string
}

func (c *collectingT) T() *testing.T { return nil }
func (c *collectingT) Errorf(format string, args ...any) {
	c.failed = true
	c.message = fmt.Sprintf(format, args...)
}
func (c *collectingT) FailNow() {
	c.failed = true
	runtime.Goexit()
}

func runCollectingPoll(fn func(poll *T)) (failed bool, message string) {
	c := &collectingT{}
	done := make(chan struct{})
	go func() {
		defer close(done)
		fn(&T{t: nil, collector: c})
	}()
	<-done
	return c.failed, c.message
}

func (t *T) Eventually(waitFor, tick time.Duration, fn func(poll *T)) {
	t.t.Helper()
	timer := time.NewTimer(waitFor)
	defer timer.Stop()
	ticker := time.NewTicker(tick)
	defer ticker.Stop()

	var lastMsg string
	polls := 0
	for {
		select {
		case <-timer.C:
			t.t.Helper()
			if lastMsg != "" {
				t.t.Fatalf("Eventually failed after %v (%d polls):\n  last failure:\n    %s", waitFor, polls, lastMsg)
			} else {
				t.t.Fatalf("Eventually failed after %v (%d polls): condition never satisfied", waitFor, polls)
			}
			return
		case <-ticker.C:
			polls++
			failed, msg := runCollectingPoll(fn)
			if !failed {
				return
			}
			lastMsg = msg
		}
	}
}

func (t *T) Consistently(waitFor, tick time.Duration, fn func(poll *T)) {
	t.t.Helper()
	timer := time.NewTimer(waitFor)
	defer timer.Stop()
	ticker := time.NewTicker(tick)
	defer ticker.Stop()

	polls := 0
	for {
		select {
		case <-timer.C:
			return
		case <-ticker.C:
			polls++
			failed, msg := runCollectingPoll(fn)
			if failed {
				t.t.Helper()
				t.t.Fatalf("Consistently failed on poll %d:\n    %s", polls, msg)
				return
			}
		}
	}
}

func (t *T) MatchSnapshot(value any, name ...string) {
	t.t.Helper()

	content := fmt.Sprintf("%v", value)
	testName := t.t.Name()

	// Determine snapshot file path
	snapName := sanitizeFilename(testName)
	if len(name) > 0 && name[0] != "" {
		snapName += "_" + sanitizeFilename(name[0])
	}
	snapDir := filepath.Join("testdata", "__snapshots__")
	snapPath := filepath.Join(snapDir, snapName+".snap")

	// Check if update mode
	if os.Getenv("GOTEST_UPDATE_SNAPSHOTS") != "" {
		if err := os.MkdirAll(snapDir, 0755); err != nil {
			t.t.Fatalf("MatchSnapshot: failed to create snapshot dir: %v", err)
			return
		}
		if err := os.WriteFile(snapPath, []byte(content), 0644); err != nil {
			t.t.Fatalf("MatchSnapshot: failed to write snapshot: %v", err)
			return
		}
		t.t.Logf("updated snapshot: %s", snapPath)
		return
	}

	// Read existing snapshot
	existing, err := os.ReadFile(snapPath)
	if err != nil {
		if os.IsNotExist(err) {
			// First run: create snapshot
			if err := os.MkdirAll(snapDir, 0755); err != nil {
				t.t.Fatalf("MatchSnapshot: failed to create snapshot dir: %v", err)
				return
			}
			if err := os.WriteFile(snapPath, []byte(content), 0644); err != nil {
				t.t.Fatalf("MatchSnapshot: failed to write snapshot: %v", err)
				return
			}
			t.t.Logf("created snapshot: %s", snapPath)
			return
		}
		t.t.Fatalf("MatchSnapshot: failed to read snapshot: %v", err)
		return
	}

	// Compare
	want := string(existing)
	if content != want {
		d := assert.Diff(want, content)
		if d != "" {
			t.t.Fatalf("MatchSnapshot: snapshot mismatch (%s):\n  diff:\n%s\nRun with GOTEST_UPDATE_SNAPSHOTS=1 to update", snapPath, d)
		} else {
			t.t.Fatalf("MatchSnapshot: snapshot mismatch (%s):\n  expected: %s\n  actual:   %s\nRun with GOTEST_UPDATE_SNAPSHOTS=1 to update", snapPath, want, content)
		}
	}
}

func sanitizeFilename(s string) string {
	s = strings.ReplaceAll(s, "/", "_")
	s = strings.ReplaceAll(s, " ", "_")
	s = strings.ReplaceAll(s, ":", "_")
	return s
}
