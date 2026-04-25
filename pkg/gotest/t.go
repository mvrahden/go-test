package gotest

import (
	"testing"

	"github.com/mvrahden/go-test/pkg/gotest/internal/assert"
)

type TestCase func(*T)

func NewT(t *testing.T) *T {
	return &T{t: t}
}

type T struct {
	t *testing.T
}

func (t *T) T() *testing.T { return t.t }
func (t *T) It(description string, testFn func(it *T)) {
	t.t.Run(description, func(t *testing.T) {
		testFn(NewT(t))
	})
}
func (t *T) Assert(v any) *assert.BaseAssertionContext {
	t.t.Helper()
	return assert.NewAssertionContext(v, t.t.Fatalf)
}
