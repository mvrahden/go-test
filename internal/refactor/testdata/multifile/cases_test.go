package multifile

import "github.com/mvrahden/go-test/pkg/gotest"

func (s *MultiTestSuite) TestTwo(t *gotest.T) { s.add(t) }

func (s *MultiTestSuite) TestThree(t *gotest.T) { s.add(t) }
