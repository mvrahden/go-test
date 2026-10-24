package protocol

import "github.com/mvrahden/go-test/pkg/gotest"

type StdoutTestSuite struct {
	Shared *StdoutSharedFixture
}

func (s *StdoutTestSuite) TestStateArrived(t *gotest.T) { gotest.Equal(t, "printed", s.Shared.Note) }

type BigTestSuite struct {
	Shared *BigSharedFixture
}

func (s *BigTestSuite) TestStateArrived(t *gotest.T) { gotest.Len(t, s.Shared.Blob, 2<<20) }

type LogTestSuite struct {
	Shared *LogSharedFixture
}

func (s *LogTestSuite) TestStateArrived(t *gotest.T) { gotest.Equal(t, "logged", s.Shared.Note) }
