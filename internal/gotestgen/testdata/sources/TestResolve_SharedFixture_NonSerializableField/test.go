package testpkg

import (
	"context"
	"github.com/mvrahden/go-test/pkg/gotest"
)

type BadSharedFixture struct {
	ConnStr string
	Notify  chan struct{}
}

func (f *BadSharedFixture) BeforeAll(ctx context.Context) error { return nil }

type MyTestSuite struct{ *BadSharedFixture }

func (s *MyTestSuite) TestOne(t *gotest.T) {}
