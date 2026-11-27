package gotest

import (
	"encoding/json"
	"os"
	"reflect"
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
		by, n := c.by.Load(), c.n.Load()
		if by != nil {
			tt.Attr(protocol.AttrAssertsBy, *by)
		}
		tt.Attr(protocol.AttrAssertions, strconv.FormatInt(n, 10))
		writeCount(tt, n, by != nil)
	})
	return c
}

// countsFile is the file a text run asked for each leaf's count in; nil
// when none was asked for, or testing's record of subtests cannot be read,
// so a test that ran one through t.T().Run would read as a leaf.
var (
	countsFile = openCountsFile()
	countsMu   sync.Mutex
)

func openCountsFile() *os.File {
	path, ok := os.LookupEnv(protocol.EnvAssertionsFile)
	_ = os.Unsetenv(protocol.EnvAssertionsFile) // the test's own children are not this run's
	if !ok || path == "" || subtestsField == nil {
		return nil
	}
	f, err := os.OpenFile(path, os.O_WRONLY|os.O_APPEND, 0) //nolint:gosec // G304: the file the runner created
	if err != nil {
		return nil
	}
	return f
}

// subtestsField is the index of testing's unexported mark that a test ran a
// subtest, nil where testing has no such field.
var subtestsField = func() []int {
	f, ok := reflect.TypeFor[testing.T]().FieldByName("hasSub")
	if !ok || f.Type != reflect.TypeFor[atomic.Bool]() {
		return nil
	}
	if v, ok := f.Type.FieldByName("v"); !ok || v.Type.Kind() != reflect.Uint32 {
		return nil
	}
	return f.Index
}()

// ranSubtests reports whether tt ran a subtest through its own Run, as the
// stream's run events show it.
func ranSubtests(tt *testing.T) bool {
	return reflect.ValueOf(tt).Elem().FieldByIndex(subtestsField).FieldByName("v").Uint() != 0
}

// writeCount appends tt's count to countsFile, one line written whole; a test
// that ran subtests is no leaf, and a failed write leaves the count unknown.
func writeCount(tt *testing.T, n int64, by bool) {
	if countsFile == nil || ranSubtests(tt) {
		return
	}
	line, _ := json.Marshal(protocol.LeafCount{Test: tt.Name(), Assertions: n, AssertsBy: by, Failed: tt.Failed()})
	countsMu.Lock()
	defer countsMu.Unlock()
	_, _ = countsFile.Write(append(line, '\n'))
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
