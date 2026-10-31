package sharedintest

import (
	"context"

	"github.com/mvrahden/go-test/pkg/gotest"
)

// PGSharedFixture lives in a test file the setup program cannot import.
type PGSharedFixture struct{ Conn string }

func (f *PGSharedFixture) BeforeAll(ctx context.Context) error { f.Conn = "pg"; return nil }

type UsesPGTestSuite struct {
	PG *PGSharedFixture
}

func (s *UsesPGTestSuite) TestConn(t *gotest.T) { gotest.Equal(t, "pg", s.PG.Conn) }
