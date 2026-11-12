//go:build fixsbuildflags

package fixsbuildflags

import "github.com/mvrahden/go-test/pkg/gotest"

type TaggedTestSuite struct {
	Tagged *TaggedSharedFixture
}

func (s *TaggedTestSuite) TestReadsTheFixture(t *gotest.T) { gotest.Equal(t, "up", s.Tagged.Addr) }
