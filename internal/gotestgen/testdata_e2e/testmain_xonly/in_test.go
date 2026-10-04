package testmainxonly

import "github.com/mvrahden/go-test/pkg/gotest"

// InTestSuite binds no fixture: the external package owns the TestMain.
type InTestSuite struct{}

func (s *InTestSuite) TestWrite(t *gotest.T) {}
