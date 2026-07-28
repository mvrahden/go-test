package testpkg

import (
	"fmt"

	"github.com/mvrahden/go-test/pkg/gotest"
)

// EachPanicTestSuite records a non-fatal failure and then panics inside the same
// Each entry. eachRun's deferred FailNow would otherwise run runtime.Goexit
// while the panic is unwinding, discarding it.
type EachPanicTestSuite struct{}

func (s *EachPanicTestSuite) AfterAll(t *gotest.T) {
	fmt.Println("MARK:suite afterall")
}

func (s *EachPanicTestSuite) TestEachFailsThenPanics(t *gotest.T) {
	for sub, entry := range gotest.Each(t, []string{"only"}) {
		sub.Errorf("recorded a non-fatal failure for %s", entry)
		panic("boom after a recorded failure")
	}
}
