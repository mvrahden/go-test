package web

import (
	"context"

	sharedfixture "github.com/mvrahden/go-test/examples/shared_fixture"
	"github.com/mvrahden/go-test/pkg/gotest"
)

type WebFixture struct {
	*sharedfixture.PostgresSharedFixture
}

func (f *WebFixture) BeforeAll(ctx context.Context) error { return nil }

type PageTestSuite struct {
	*WebFixture
}

func (s *PageTestSuite) TestRenderHome(t *gotest.T) {}
