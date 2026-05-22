package testpkg

import (
	"context"
	"github.com/mvrahden/go-test/pkg/gotest"
)

type InfraFixture struct{ Val string }
func (f *InfraFixture) BeforeAll(ctx context.Context) error { return nil }

type APIFixture struct { infra *InfraFixture }
func (f *APIFixture) BeforeAll(ctx context.Context) error { return nil }

type LightTestSuite struct { *InfraFixture }
func (s *LightTestSuite) TestOne(t *gotest.T) {}

type FullTestSuite struct { *APIFixture }
func (s *FullTestSuite) TestOne(t *gotest.T) {}
