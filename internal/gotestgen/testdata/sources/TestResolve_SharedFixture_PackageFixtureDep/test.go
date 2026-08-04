package testpkg

import (
	"context"

	"github.com/mvrahden/go-test/pkg/gotest"
)

type LocalFixture struct{}

func (f *LocalFixture) BeforeAll(ctx context.Context) error { return nil }

type MixedSharedFixture struct {
	Local *LocalFixture
}

func (f *MixedSharedFixture) BeforeAll(ctx context.Context) error { return nil }

type MixedTestSuite struct {
	Mixed *MixedSharedFixture
}

func (s *MixedTestSuite) TestOne(t *gotest.T) {}
