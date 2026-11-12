package sharedifaceok

import "github.com/mvrahden/go-test/pkg/gotest"

type ChainTestSuite struct{ Chain *ChainSharedFixture }

func (s *ChainTestSuite) TestState(t *gotest.T) {
	gotest.Equal(t, int64(7), s.Chain.Clock.Now())
	gotest.Equal(t, "b", s.Chain.Chain.Next.Name)
	gotest.Equal(t, any("free"), s.Chain.Any)
}
