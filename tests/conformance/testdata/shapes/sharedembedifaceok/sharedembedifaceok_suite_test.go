package sharedembedifaceok

import "github.com/mvrahden/go-test/pkg/gotest"

type HandleTestSuite struct{ Handle *HandleSharedFixture }

func (s *HandleTestSuite) TestHandle(t *gotest.T) {
	gotest.Equal(t, int64(7), s.Handle.Now())
	gotest.Equal(t, Endpoint("http://x"), s.Handle.Endpoint)
}
