package togglespread

import "github.com/mvrahden/go-test/pkg/gotest"

type SpreadTestSuite struct{ n int }

func (s *SpreadTestSuite) BeforeEach(t *gotest.T) { s.n = 1 }

func (s *SpreadTestSuite) TestOne(t *gotest.T) { gotest.Equal(t, 1, s.n) }
