package multifile

import "github.com/mvrahden/go-test/pkg/gotest"

type MultiTestSuite struct{ count int }

func (s *MultiTestSuite) BeforeEach(t *gotest.T) { s.count = 0 }

func (s *MultiTestSuite) TestOne(t *gotest.T) { s.add(t) }
