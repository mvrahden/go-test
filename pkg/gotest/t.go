package gotest

import (
	"testing"

	"github.com/mvrahden/go-test/pkg/asserter"
)

type TestCase func(*T)

func NewT(t *testing.T) *T {
	return &T{t: t, Assert: asserter.NewAsserter(t)}
}

type T struct {
	t      *testing.T
	Assert asserter.Asserter
}

func (t *T) It(description string, testFn func(it *T))  {}
func (t *T) XIt(description string, testFn func(it *T)) {} // skips
func (t *T) FIt(description string, testFn func(it *T)) {} // skips
func (t *T) ItAsync(description string, testFn func(it *T, done func())) {
	doneC := make(chan struct{}, 1)
	done := func() {
		doneC <- struct{}{}
	}
	testFn(nil /*TODO*/, done)
}
func (t *T) XItAsync(description string, testFn func(it *T, done func())) {} // skips
func (t *T) FItAsync(description string, testFn func(it *T, done func())) {} // skips
