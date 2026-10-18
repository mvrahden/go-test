package sigresult

import "github.com/mvrahden/go-test/pkg/gotest"

type XTestSuite struct{}

func (s *XTestSuite) TestGood(t *gotest.T) {}

func (s *XTestSuite) TestBad(t *gotest.T) error { return nil }
