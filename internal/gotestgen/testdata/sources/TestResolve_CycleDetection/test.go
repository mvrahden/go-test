package testpkg

import (
	"context"
	"github.com/mvrahden/go-test/pkg/gotest"
)

type AFixture struct{ BFixture *BFixture }

func (f *AFixture) BeforeAll(ctx context.Context) error { return nil }

type BFixture struct{ AFixture *AFixture }

func (f *BFixture) BeforeAll(ctx context.Context) error { return nil }

type SomeTestSuite struct{ AFixture *AFixture }

func (s *SomeTestSuite) TestOne(t *gotest.T) {}
