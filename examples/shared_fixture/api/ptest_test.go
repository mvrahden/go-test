package api

import (
	"context"

	sharedfixture "github.com/mvrahden/go-test/examples/shared_fixture"
	"github.com/mvrahden/go-test/pkg/gotest"
)

type APIFixture struct {
	*sharedfixture.PostgresSharedFixture
	*sharedfixture.RedisSharedFixture
}

func (f *APIFixture) BeforeAll(ctx context.Context) error { return nil }

type UserTestSuite struct {
	*APIFixture
}

func (s *UserTestSuite) TestCreateUser(t *gotest.T) {}
func (s *UserTestSuite) TestListUsers(t *gotest.T)  {}
