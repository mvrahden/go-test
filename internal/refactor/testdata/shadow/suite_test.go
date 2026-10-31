package shadow

import "github.com/mvrahden/go-test/pkg/gotest"

type ShadowTestSuite struct{}

func (s *ShadowTestSuite) TestOne(t *gotest.T) {}

type OtherTestSuite struct{ ShadowTestSuite int }

func (s *OtherTestSuite) TestOne(t *gotest.T) { s.ShadowTestSuite++ }
