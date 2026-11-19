package sel_test

import "github.com/mvrahden/go-test/pkg/gotest"

// SelTestSuite is the suite a filtered run names.
type SelTestSuite struct{}

func (s *SelTestSuite) TestPasses(t *gotest.T) { gotest.True(t, true) }
