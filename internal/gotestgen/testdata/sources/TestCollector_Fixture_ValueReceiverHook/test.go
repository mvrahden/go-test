package testpkg

import (
	"context"

	"github.com/mvrahden/go-test/pkg/gotest"
)

type SloppyFixture struct{}

func (f SloppyFixture) BeforeAll(ctx context.Context) error { return nil }

type SloppyTestSuite struct {
	Fixture *SloppyFixture
}

func (s *SloppyTestSuite) TestOne(t *gotest.T) {}
