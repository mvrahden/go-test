// Package lcpanicahead panics ahead of an AfterEach that fails.
package lcpanicahead

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
