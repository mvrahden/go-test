package customflag_test

import (
	"flag"

	"github.com/mvrahden/go-test/pkg/gotest"
)

// customflag is a flag only the test binary knows; a run passes when the
// command line delivered it, so the suite tells whether a flag gotest does
// not recognise reached the binary.
var customflag = flag.String("customflag", "", "a flag only this test binary declares")

type CustomFlagTestSuite struct{}

func (s *CustomFlagTestSuite) TestSeesTheFlag(t *gotest.T) {
	gotest.Equal(t, "yes", *customflag)
}
