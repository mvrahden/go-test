package testpkg

import (
	"context"
	"github.com/mvrahden/go-test/pkg/gotest"
)

type DBFixture struct{}
func (f *DBFixture) BeforeAll(ctx context.Context) error { return nil }

type UserRepoFixture struct { DB *DBFixture }
func (f *UserRepoFixture) BeforeAll(ctx context.Context) error { return nil }

type OrderRepoFixture struct { DB *DBFixture }
func (f *OrderRepoFixture) BeforeAll(ctx context.Context) error { return nil }

type IntegrationTestSuite struct {
	Users  *UserRepoFixture
	Orders *OrderRepoFixture
}
func (s *IntegrationTestSuite) TestOne(t *gotest.T) {}
