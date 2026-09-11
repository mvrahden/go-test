package repository_test

import "github.com/mvrahden/go-test/pkg/gotest"

type UserRepositoryTestSuite struct{}

func (s *UserRepositoryTestSuite) SuiteConfig() gotest.SuiteConfig {
	cfg := gotest.DefaultSuiteConfig()
	cfg.Parallel = true
	return cfg
}

type usersCtx struct {
	users map[string]string
}

func (s *UserRepositoryTestSuite) BeforeEach(t *gotest.T) *usersCtx {
	return &usersCtx{users: map[string]string{}}
}

func (s *UserRepositoryTestSuite) TestCreateUser(t *gotest.T, ctx *usersCtx) {
	t.When("a user is created", func(t *gotest.T) {
		ctx.users["alice"] = "alice@example.com"

		t.It("stores the email", func(t *gotest.T) {
			gotest.Equal(t, "alice@example.com", ctx.users["alice"])
		})
		t.It("has one entry", func(t *gotest.T) {
			gotest.Len(t, ctx.users, 1)
		})
	})
}

func (s *UserRepositoryTestSuite) TestDeleteUser(t *gotest.T, ctx *usersCtx) {
	t.When("the only user is removed", func(t *gotest.T) {
		ctx.users["bob"] = "bob@example.com"
		delete(ctx.users, "bob")

		t.It("leaves the store empty", func(t *gotest.T) {
			gotest.Empty(t, ctx.users)
		})
	})
}
