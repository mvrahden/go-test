package withgeneration

import "github.com/mvrahden/go-test/pkg/gotest"

// ResultTestSuite has a case returning a value, and a case with a value
// receiver, which the receiver rule reports.
type ResultTestSuite struct{}

func (s *ResultTestSuite) TestLoads(t *gotest.T) error { return nil } // want `TestLoads on ResultTestSuite returns a value; a Test method returns nothing`

func (s ResultTestSuite) TestByValue(t *gotest.T) {} // want `suite method ResultTestSuite.TestByValue should use a pointer receiver`

func (s *ResultTestSuite) TestSuppressed(t *gotest.T) int { return 0 } //nolint:generation-rejects
