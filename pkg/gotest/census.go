package gotest

import (
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/mvrahden/go-test/internal/protocol"
)

// census counts the assertions one test makes. A test that opens a subtest
// is no leaf, and its count is not reported.
type census struct {
	n      atomic.Int64
	nested atomic.Bool
	by     atomic.Pointer[string] // the reason AssertsBy declared
}

var (
	// censuses holds the census of every running test that has one, by its
	// *testing.T, so each T wrapping that test counts into it.
	censuses sync.Map
	// openCensuses is how many are held: none, and a lookup costs a load.
	openCensuses atomic.Int64
)

// openCensus counts the assertions of tt until it ends, then reports them
// as tt's attributes unless tt opened a subtest or was skipped. by is the
// reason the test above declared, nil for none.
func openCensus(tt *testing.T, by *string) *census {
	c := &census{}
	c.by.Store(by)
	censuses.Store(tt, c)
	openCensuses.Add(1)
	tt.Cleanup(func() {
		censuses.Delete(tt)
		openCensuses.Add(-1)
		if c.nested.Load() || tt.Skipped() {
			return
		}
		if by := c.by.Load(); by != nil {
			tt.Attr(protocol.AttrAssertsBy, *by)
		}
		tt.Attr(protocol.AttrAssertions, strconv.FormatInt(c.n.Load(), 10))
	})
	return c
}

// censusOf is the census of tb's test; nil when it has none.
func censusOf(tb *testing.T) *census {
	if tb == nil || openCensuses.Load() == 0 {
		return nil
	}
	if c, ok := censuses.Load(tb); ok {
		return c.(*census) //nolint:forcetypeassert // only openCensus stores
	}
	return nil
}

// census is the census t counts into: the one its test had when t was made,
// else the one it has now.
func (t *T) census() *census {
	if t.c != nil {
		return t.c
	}
	return censusOf(t.t)
}

// countAssertion counts one assertion made on t. One on a poll's *R counts nothing:
// the poll counted once on its own T.
func countAssertion(t testingT) {
	var c *census
	switch v := t.(type) {
	case *T:
		c = v.census()
	case *testing.T:
		c = censusOf(v)
	}
	if c != nil {
		c.n.Add(1)
	}
}

// opens marks t's test as one that opens subtests: no leaf.
func (t *T) opens() {
	if c := t.census(); c != nil {
		c.nested.Store(true)
	}
}

// AssertsBy declares that the test checks its outcome by other means than a
// gotest assertion — code given T() that fails it, a raw check, a call that
// must not panic — so a pass that made no assertion is not vacuous. reason
// says which; it is shown with the test. It holds for t's test and for the
// tests opened below it afterwards. An empty reason fails the test.
func (t *T) AssertsBy(reason string) {
	reason = strings.Join(strings.Fields(reason), " ")
	if reason == "" {
		fail(t, "AssertsBy: the reason is empty; say how the test asserts", nil)
		return
	}
	if c := t.census(); c != nil {
		c.by.Store(&reason)
		return
	}
	t.by = &reason // a T whose test counts nothing still hands it down
}

// declared is the reason t's test declared, nil for none.
func (t *T) declared() *string {
	if c := t.census(); c != nil {
		return c.by.Load()
	}
	return t.by
}
