// Package hookpanics panics ahead of an AfterEach that fails, for a test
// method and a fuzz execution.
package hookpanics

import (
	"fmt"

	"github.com/mvrahden/go-test/pkg/gotest"
)

// PanicAheadTestSuite: BeforeEach panics, then AfterEach fails a fatal
// assertion; the panic must still be reported.
type PanicAheadTestSuite struct{}

func (s *PanicAheadTestSuite) BeforeEach(t *gotest.T) { panic("TEST-BEFORE-EACH-PANIC") }
func (s *PanicAheadTestSuite) AfterEach(t *gotest.T) {
	fmt.Println("MARK after the panic")
	gotest.Fail(t, "AfterEach fails")
}
func (s *PanicAheadTestSuite) TestOne(t *gotest.T) {}

// FuzzPanicAheadTestSuite: the same, around a fuzz execution.
type FuzzPanicAheadTestSuite struct{}

func (s *FuzzPanicAheadTestSuite) BeforeEach(t *gotest.T) { panic("FUZZ-BEFORE-EACH-PANIC") }
func (s *FuzzPanicAheadTestSuite) AfterEach(t *gotest.T) {
	fmt.Println("MARK after the fuzz panic")
	gotest.Fail(t, "AfterEach fails")
}
func (s *FuzzPanicAheadTestSuite) FuzzOne(f *gotest.F) {
	f.Add("seed")
	f.Fuzz(func(t *gotest.T, in string) {})
}
