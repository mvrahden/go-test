package b

import "github.com/mvrahden/go-test/pkg/gotest"

// OtherTestSuite focuses nothing and fails when it runs.
type OtherTestSuite struct{}

func (s *OtherTestSuite) TestFails(t *gotest.T) { gotest.Fail(t, "an unfocused suite ran") }
