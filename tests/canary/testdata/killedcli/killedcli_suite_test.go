package killedcli

import "github.com/mvrahden/go-test/pkg/gotest"

// KeepTestSuite holds until its context ends, so the CLI can be killed while
// the fixture is up.
type KeepTestSuite struct {
	Keep *KeepSharedFixture
}

func (s *KeepTestSuite) TestHolds(t *gotest.T) {
	mark("suite holds")
	<-t.Context().Done()
}
