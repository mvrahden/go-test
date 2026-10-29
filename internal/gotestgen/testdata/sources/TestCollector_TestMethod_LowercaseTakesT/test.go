package testpkg

import (
	"testing"

	"github.com/mvrahden/go-test/pkg/gotest"
)

type NameTestSuite struct{}

func (s *NameTestSuite) TestRuns(t *gotest.T)         {}
func (s *NameTestSuite) Testcreate(t *gotest.T)       {}
func (s *NameTestSuite) F_Benchmarkfast(b *testing.B) {}
