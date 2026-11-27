package gotest

import (
	"bytes"
	"context"
	"fmt"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"testing/synctest"

	"github.com/mvrahden/go-test/internal/setupwaits"
)

// Bubble runs fn in a testing/synctest bubble, on a T of t's own test: time
// in it is a fake clock that moves only when every goroutine in the bubble
// is durably blocked, so a backoff, ticker, TTL or context deadline runs at
// once and the same under any load. Bubble returns when fn and every
// goroutine started in the bubble have returned.
//
// Assertions on fn's T fail t's test at their lines, an Each row's going on,
// and count for it; a skip skips it. Its Context is the bubble's: it has no
// deadline and ends when fn returns, before Bubble waits for the goroutines
// gotest.Go started on it. Its Cleanup, TempDir and Setenv end with the
// bubble. Eventually and Consistently on it poll on the fake clock.
// The test's Timeout bounds the real time the bubble takes, never its fake
// time: when t's context ends first, Bubble fails the test and returns,
// leaving the bubble's goroutines to run on, as Go cannot stop them.
//
// What cannot work in a bubble fails the test at the call: It, When, Each
// and Bubble on fn's T, Run, Parallel and Deadline on its T(), and Context,
// Go, It, When, Each or Bubble on a T from outside the bubble. A goroutine
// started before the bubble is not in it, and a channel, timer or ticker
// made in it must not be used after it. A bubble whose goroutines all block
// with no timer due fails the test at the Bubble, listing them.
func Bubble(t *T, fn func(bt *T)) {
	switch {
	case t.t == nil:
		fail(t, "Bubble is unavailable in benchmark and fuzz lifecycle hooks: they have no *testing.T", nil)
		return
	case t.bubbled:
		fail(t, "Bubble inside a bubble: bubbles do not nest; call Bubble once, outside", nil)
		return
	}
	t.refuseInBubble("Bubble")
	ctx := t.Context()
	activeBubbles.Add(1)
	defer activeBubbles.Add(-1)

	out := &bubbleRun{}
	done := make(chan struct{})
	go func() {
		defer close(done)
		defer func() { out.panicked = recover() }()
		synctest.Test(t.t, func(tt *testing.T) { out.run(t, tt, fn) })
		out.passed = true
	}()
	select {
	case <-done:
	case <-ctx.Done():
		select {
		case <-done: // it ended as the context did
		default:
			fail(t, fmt.Sprintf("Bubble abandoned when the test's context ended (%v): a goroutine in it is blocked on something outside the bubble, such as a mutex, I/O or a channel made outside, and its fake clock cannot move", context.Cause(ctx)), nil)
			return
		}
	}
	out.release()

	if p := out.panicked; p != nil {
		if reason, ok := deadlockReason(p); ok {
			fail(t, "Bubble "+reason+": each goroutine left waits for another in the bubble and no timer is due; blocked:\n"+out.blocked, nil)
			return
		}
		if s, ok := p.(string); ok && strings.Contains(s, "within a synctest bubble") {
			fail(t, "Bubble inside a synctest bubble: bubbles do not nest; call Bubble once, outside", nil)
			return
		}
		panic(p)
	}
	if out.carried != nil {
		panic(out.carried) // on the test's goroutine, where testing reports it
	}
	switch {
	case out.skipped:
		t.testingTB().SkipNow()
	case !out.passed && !t.soft:
		t.FailNow() // fn failed it: synctest.Test failed t on another goroutine
	}
}

// Settle waits until every goroutine in t's bubble other than the caller is
// durably blocked: what they wrote before blocking is then safe to read. It
// fails the test when t is not a bubble's T, or when another goroutine of
// the bubble is settling.
func Settle(t *T) {
	if !t.bubbled {
		fail(t, "Settle runs only in a bubble: call it on the T Bubble gives its callback", nil)
		return
	}
	if p := settle(); p != nil {
		fail(t, fmt.Sprintf("Settle: %v: only one goroutine of a bubble may settle at a time", p), nil)
	}
}

// settle is synctest.Wait, returning what it panicked with.
func settle() (p any) {
	defer func() { p = recover() }()
	synctest.Wait()
	return nil
}

// bubbleRun is what one bubble left for the Bubble that opened it.
type bubbleRun struct {
	inner    atomic.Pointer[testing.T]
	counted  atomic.Bool
	passed   bool           // synctest.Test returned: fn did not fail the test
	skipped  bool           // fn skipped
	panicked any            // what synctest.Test itself panicked with
	carried  *capturedPanic // what fn panicked with
	blocked  string         // the bubble's goroutines, when it deadlocked
	id       atomic.Int64
}

// run runs fn on the bubble's T, in the bubble.
func (out *bubbleRun) run(t *T, tt *testing.T, fn func(*T)) {
	out.id.Store(bubbleID())
	out.inner.Store(tt)
	bubbleTs.Store(tt, struct{}{})
	c := t.census()
	if c != nil {
		censuses.Store(tt, c)
		openCensuses.Add(1)
		out.counted.Store(true)
	}
	// The callback's context ends when it returns, before the waits on its
	// gotest.Go goroutines, as a method's ends before its cleanups.
	ctx, cancel := context.WithCancel(tt.Context())
	defer cancel()
	waits := &setupwaits.Set{TB: tt}
	bt := &T{t: tt, tb: tt, ctx: setupwaits.With(ctx, waits), soft: t.soft, c: c, by: t.by, bubbled: true}
	returned := false
	defer func() {
		r := recover()
		switch {
		case r == nil && returned:
		case r == nil: // a Goexit
			out.skipped = tt.Skipped()
			if !tt.Failed() && !tt.Skipped() {
				refuse(tt, userLine(), "FailNow or SkipNow on a T from outside the bubble: inside Bubble use the T its callback is given")
			}
		default:
			if msg, at, ok := refusalOf(r); ok {
				refuse(tt, at, msg)
			}
			out.carried = capturedFrom(r, "Bubble callback")
		}
	}()
	execTestFn(fn, bt)
	cancel()
	for ws := waits.Take(); len(ws) > 0; ws = waits.Take() {
		for _, w := range ws {
			w()
		}
	}
	out.skipped = tt.Skipped()
	returned = true
}

// release forgets the bubble's T; on a deadlock it also lists the
// goroutines the bubble left blocked.
func (out *bubbleRun) release() {
	if tt := out.inner.Load(); tt != nil {
		bubbleTs.Delete(tt)
		if out.counted.Load() {
			censuses.Delete(tt)
			openCensuses.Add(-1)
		}
	}
	if _, ok := deadlockReason(out.panicked); ok {
		out.blocked = bubbleStacks(out.id.Load())
	}
}

var (
	// activeBubbles counts the bubbles running: none, and a T's check costs a load.
	activeBubbles atomic.Int64
	// bubbleTs holds the *testing.T of every running bubble.
	bubbleTs sync.Map
)

// isBubbleT reports whether tt is a running bubble's.
func isBubbleT(tt *testing.T) bool {
	if tt == nil || activeBubbles.Load() == 0 {
		return false
	}
	_, ok := bubbleTs.Load(tt)
	return ok
}

// refuseInBubble refuses what on t: t is no bubble's, and the caller runs in
// a bubble, where it would mix the bubble's fake clock and channels with the
// test's real ones.
func (t *T) refuseInBubble(what string) {
	if activeBubbles.Load() == 0 || t.bubbled || t.t == nil || !inBubble() {
		return
	}
	panic(&bubbleRefusal{msg: what + " on a T from outside the bubble: inside Bubble use the T its callback is given", at: userLine()})
}

// refuseBubbled fails t when it is a bubble's: what opens a subtest, which a
// bubble cannot run.
func (t *T) refuseBubbled(what string) bool {
	if !t.bubbled {
		return false
	}
	fail(t, what+" inside a Bubble: a bubble runs no subtest; open it outside and call Bubble in its body", nil)
	return true
}

// bubbleRefusal is the panic a refused call raises in a bubble; the Bubble
// fails the test with it.
type bubbleRefusal struct{ msg, at string }

func (r *bubbleRefusal) String() string { return r.at + ": " + r.msg }

// refusalOf reads what a bubble's callback panicked with as a refusal: one
// of gotest's, or testing's for Run, Parallel and Deadline.
func refusalOf(r any) (msg, at string, ok bool) {
	if c, isCarried := r.(*capturedPanic); isCarried {
		r = c.Unwrap()
	}
	switch v := r.(type) {
	case *bubbleRefusal:
		return v.msg, v.at, true
	case string:
		if strings.HasPrefix(v, "testing: ") && strings.HasSuffix(v, " inside synctest bubble") {
			return strings.TrimPrefix(v, "testing: ") + ": Run, Parallel and Deadline cannot run in a bubble", userLine(), true
		}
	}
	return "", "", false
}

// refuse fails tt at the user's line at, with msg, and ends it.
func refuse(tt *testing.T, at, msg string) {
	if at != "" {
		msg = at + ": " + msg
	}
	fmt.Fprintln(tt.Output(), msg)
	tt.FailNow()
}

// userLine is the innermost line of the user's on the stack, "" for none.
func userLine() string {
	pcs := make([]uintptr, 64)
	frames := runtime.CallersFrames(pcs[:runtime.Callers(2, pcs)])
	for {
		f, more := frames.Next()
		if userFrame(&f) {
			return formatFrame(&f)
		}
		if !more {
			return ""
		}
	}
}

// deadlockReason is the reason synctest gave for a bubble whose goroutines
// all blocked.
func deadlockReason(p any) (string, bool) {
	err, ok := p.(error)
	if !ok || !strings.HasPrefix(err.Error(), "deadlock: ") {
		return "", false
	}
	return err.Error(), true
}

// bubbleMark is how a traceback header marks a goroutine in a bubble.
const bubbleMark = ", synctest bubble "

// inBubble reports whether the calling goroutine runs in a bubble, as its
// traceback header says; false where the header does not say.
func inBubble() bool {
	return bubbleID() != 0
}

// bubbleID is the id of the calling goroutine's bubble, 0 for none.
func bubbleID() int64 {
	var buf [128]byte
	header, _, _ := bytes.Cut(buf[:runtime.Stack(buf[:], false)], []byte("\n"))
	_, rest, ok := bytes.Cut(header, []byte(bubbleMark))
	if !ok {
		return 0
	}
	digits, _, _ := bytes.Cut(rest, []byte("]"))
	id, _ := strconv.ParseInt(string(digits), 10, 64)
	return id
}

// bubbleStacks names, for each goroutine of bubble id, what it blocks on and
// the line of the user's it blocks at; one with no such line is left out.
func bubbleStacks(id int64) string {
	if id == 0 {
		return ""
	}
	buf := make([]byte, 1<<20)
	all := string(buf[:runtime.Stack(buf, true)])
	mark := bubbleMark + strconv.FormatInt(id, 10) + "]"
	var lines []string
	for g := range strings.SplitSeq(all, "\n\n") {
		header, frames, _ := strings.Cut(g, "\n")
		if !strings.Contains(header, mark) {
			continue
		}
		if at := blockedAt(frames); at != "" {
			lines = append(lines, "  "+strings.Replace(strings.TrimSuffix(header, ":"), mark, "]", 1)+" at "+at)
		}
	}
	return strings.Join(lines, "\n")
}

// blockedAt is the innermost user frame of a traceback's frames, "" for none.
func blockedAt(frames string) string {
	rows := strings.Split(frames, "\n")
	for i := 0; i+1 < len(rows); i += 2 {
		fn, at := rows[i], strings.TrimSpace(rows[i+1])
		if strings.HasPrefix(fn, "created by ") {
			return ""
		}
		if open := strings.LastIndex(fn, "("); open > 0 {
			fn = fn[:open]
		}
		at, _, _ = strings.Cut(at, " +0x")
		colon := strings.LastIndex(at, ":")
		if colon < 0 {
			continue
		}
		line, err := strconv.Atoi(at[colon+1:])
		if err != nil {
			continue
		}
		if f := (runtime.Frame{Function: fn, File: at[:colon], Line: line}); userFrame(&f) {
			return formatFrame(&f)
		}
	}
	return ""
}
