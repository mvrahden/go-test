package b

import (
	"context"

	"github.com/mvrahden/go-test/pkg/gotest"
)

// MarkFixture records that it came up.
type MarkFixture struct{ State string }

func (f *MarkFixture) BeforeAll(ctx context.Context) error {
	mark("fixture up")
	f.State = "up"
	return nil
}

func (f *MarkFixture) AfterAll(ctx context.Context) error { return nil }

// OtherTestSuite is unfocused and binds both fixtures.
type OtherTestSuite struct {
	Local  *MarkFixture
	Shared *MarkSharedFixture
}

func (s *OtherTestSuite) TestRuns(t *gotest.T) {
	gotest.Equal(t, "up", s.Local.State)
	gotest.Equal(t, "up", s.Shared.State)
}
