package testpkg

import (
	"testing"

	"github.com/mvrahden/go-test/pkg/gotest"
)

type MixedTestSuite struct{}

func (s *MixedTestSuite) TestStdlib(t *testing.T) {}
func (s *MixedTestSuite) TestGotest(t *gotest.T)  {}
